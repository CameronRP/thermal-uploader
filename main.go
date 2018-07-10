// Copyright 2017 The Cacophony Project. All rights reserved.

package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	cptv "github.com/TheCacophonyProject/go-cptv"
	arg "github.com/alexflint/go-arg"
	"github.com/rjeczalik/notify"
)

const cptvGlob = "*.cptv"
const failedUploadsDir = "failed-uploads"

var version = "No version provided"

type Args struct {
	ConfigFile string `arg:"-c,--config" help:"path to configuration file"`
}

func (Args) Version() string {
	return version
}

func procArgs() Args {
	var args Args
	args.ConfigFile = "/etc/thermal-uploader.yaml"
	arg.MustParse(&args)
	return args
}

func main() {
	err := runMain()
	if err != nil {
		log.Fatal(err.Error())
	}
}

func runMain() error {
	log.SetFlags(0) // Removes default timestamp flag

	args := procArgs()
	log.Printf("running version: %s", version)
	conf, err := ParseConfigFile(args.ConfigFile)
	if err != nil {
		return fmt.Errorf("configuration error: %v", err)
	}
	privConfigFilename := genPrivConfigFilename(args.ConfigFile)
	log.Println("private settings file:", privConfigFilename)
	password, err := ReadPassword(privConfigFilename)
	if err != nil {
		return err
	}
	api, err := NewAPI(conf.ServerURL, conf.Group, conf.DeviceName, password)
	if err != nil {
		return err
	}
	if api.JustRegistered() {
		log.Println("first time registration - saving password")
		err := WritePassword(privConfigFilename, api.Password())
		if err != nil {
			return err
		}
	}
	bushnetAPI := NewBushnetAPI(conf.BushnetServerName, conf.BushnetServerPort)

	log.Println("making failed uploads directory")
	os.MkdirAll(filepath.Join(conf.Directory, failedUploadsDir), 0755)

	tryUploadEvent, err := makeTryUploadEvent(conf.Directory, bushnetAPI)
	if err != nil {
		return err
	}
	for {
		// Check for files to upload first in case there are CPTV
		// files around when the uploader starts.
		if err := uploadFiles(api, bushnetAPI, conf.Directory); err != nil {
			return err
		}
		// Block until there's activity in the directory. We don't
		// care what it is as uploadFiles will only act on CPTV
		// files.
		<-tryUploadEvent
	}
	return nil
}

func makeTryUploadEvent(fileDirectory string, bushnetAPI *BushnetAPI) (chan bool, error) {
	tryUploadEvent := make(chan bool)
	// Addind event when the bushnet server is detected
	go func() {
		for {
			time.Sleep(10 * time.Second)
			if bushnetAPI.IsAvailable() {
				log.Println("found bushnet server")
				tryUploadEvent <- true
				for bushnetAPI.IsAvailable() {
					time.Sleep(10 * time.Second)
				}
				log.Println("lost bushnet server")
			}
		}
	}()

	// Adding event when new file is detected
	fsEvents := make(chan notify.EventInfo, 1)
	if err := notify.Watch(fileDirectory, fsEvents, notify.InCloseWrite, notify.InMovedTo); err != nil {
		return nil, err
	}
	log.Println("watching", fileDirectory)
	go func() {
		defer notify.Stop(fsEvents)
		for {
			<-fsEvents
			tryUploadEvent <- true
		}
	}()

	return tryUploadEvent, nil
}

func genPrivConfigFilename(confFilename string) string {
	dirname, filename := filepath.Split(confFilename)
	bareFilename := strings.TrimSuffix(filename, ".yaml")
	return filepath.Join(dirname, bareFilename+"-priv.yaml")
}

func uploadFiles(api *CacophonyAPI, bushnetAPI *BushnetAPI, directory string) error {
	matches, _ := filepath.Glob(filepath.Join(directory, cptvGlob))
	for _, filename := range matches {
		err := uploadFileWithRetries(api, bushnetAPI, filename)
		if err != nil {
			return err
		}
	}
	return nil
}

func uploadFileWithRetries(api *CacophonyAPI, bushnetAPI *BushnetAPI, filename string) error {
	log.Printf("uploading: %s", filename)

	info, err := extractCPTVInfo(filename)
	if err != nil {
		log.Println("failed to extract CPTV info from file. Deleting CPTV file")
		return os.Remove(filename)
	}
	log.Printf("ts=%s duration=%ds", info.timestamp, info.duration)

	for remainingTries := 2; remainingTries >= 0; remainingTries-- {
		err := uploadFile(api, bushnetAPI, filename, info)
		if err == nil {
			log.Printf("upload complete: %s", filename)
			os.Remove(filename)
			return nil
		}
		if remainingTries >= 1 {
			log.Printf("upload failed, trying %d more times", remainingTries)
		}
	}
	log.Printf("upload failed multiple times, moving file to failed uploads folder")
	dir, name := filepath.Split(filename)
	return os.Rename(filename, filepath.Join(dir, failedUploadsDir, name))
}

func uploadFile(api *CacophonyAPI, bushnetAPI *BushnetAPI, filename string, info *cptvInfo) error {
	f, err := os.Open(filename)
	if os.IsNotExist(err) {
		// File disappeared since the event was generated. Ignore.
		return nil
	} else if err != nil {
		return err
	}
	defer f.Close()
	if bushnetAPI.IsAvailable() {
		return bushnetAPI.UploadFile(bufio.NewReader(f))
	}
	return api.UploadThermalRaw(info, bufio.NewReader(f))
}

type cptvInfo struct {
	timestamp time.Time
	duration  int
}

func extractCPTVInfo(filename string) (*cptvInfo, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// TODO: use the higher level cptv.Reader type (when it exists!)
	p, err := cptv.NewParser(bufio.NewReader(file))
	if err != nil {
		return nil, err
	}
	fields, err := p.Header()
	if err != nil {
		return nil, err
	}
	timestamp, err := fields.Timestamp(cptv.Timestamp)
	if err != nil {
		return nil, err
	}

	frames := 0
	for {
		_, _, err := p.Frame()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		frames++
	}
	return &cptvInfo{
		timestamp: timestamp,
		duration:  frames / 9,
	}, nil
}
