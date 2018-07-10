package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// BushnetAPI api for connecting to the local bushnet server
type BushnetAPI struct {
	localName string
	port      int
	serverURL string
	state     string
}

// NewBushnetAPI gets a new BushnetAPI object
func NewBushnetAPI(localName string, port int) *BushnetAPI {
	bushnetAPI := &BushnetAPI{
		localName: localName,
		port:      port,
		serverURL: "",
		state:     "FINISHED_UPLOADING",
	}
	bushnetAPI.startSignal()
	return bushnetAPI
}

func (api *BushnetAPI) startSignal() {
	go func() {
		for {
			if api.IsAvailable() {
				switch api.state {
				case "FINISHED_UPLOADING":
					api.sendIsFinished()
					break
				case "UPLOADING":
					api.sendIsUploading()
					break
				}
			}
			time.Sleep(5 * time.Second)
		}
	}()
}

// UploadFile uploads a file to the Bushnet server
func (api *BushnetAPI) UploadFile(r io.Reader) error {
	api.sendIsUploading()
	buf := new(bytes.Buffer)
	w := multipart.NewWriter(buf)
	fw, err := w.CreateFormFile("cptv", "file")
	if err != nil {
		return err
	}
	io.Copy(fw, r)

	w.Close()

	url := fmt.Sprintf("http://%s:%d/upload_cptv", api.serverURL, api.port)
	req, err := http.NewRequest("POST", url, buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		bodyString := string(bodyBytes)
		log.Printf("status code: %d, body:\n%s", resp.StatusCode, bodyString)
		return errors.New("non 200 status code from local server")
	}
	return nil
}

// IsAvailable will return true if it can find a bushnet server on the local network
func (api *BushnetAPI) IsAvailable() bool {
	out, err := exec.Command("avahi-resolve", "-4", "-n", api.localName).Output()
	if err != nil {
		log.Println(err)
		return false
	}
	s := string(out)
	if strings.Contains(s, "Failed to resolve host name") {
		return false
	}
	sl := strings.Split(s, "\t")
	if len(sl) != 2 {
		return false
	}
	s = strings.Split(s, "\t")[1]
	api.serverURL = strings.TrimSuffix(s, "\n")
	url := fmt.Sprintf("http://%s:%d", api.serverURL, api.port)
	res, err := http.Get(url)
	if err != nil {
		return false
	}
	res.Body.Close()
	return true
}

func (api *BushnetAPI) sendIsUploading() {
	url := fmt.Sprintf("http://%s:%d/uploading", api.serverURL, api.port)
	res, err := http.Get(url)
	if err != nil {
		log.Println(err)
		return
	}
	res.Body.Close()
}

func (api *BushnetAPI) sendIsFinished() {
	url := fmt.Sprintf("http://%s:%d/finished", api.serverURL, api.port)
	res, err := http.Get(url)
	if err != nil {
		log.Println(err)
		return
	}
	res.Body.Close()
}
