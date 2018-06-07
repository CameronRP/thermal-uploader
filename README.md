# thermal-uploader

This software is used by The Cacophony Project to upload thermal video
recordings in CPTV format to the project's API server. These
recordings are typically created by the
[thermal-recorder](https://github.com/TheCacophonyProject/thermal-recorder/).

## Releases

This software uses the [GoReleaser](https://goreleaser.com) tool to
automate releases. To produce a release:

* Ensure that the `GITHUB_TOKEN` environment variable is set with a
  Github personal access token which allows access to the Cacophony
  Project repositories.
* Tag the release with an annotated tag. For example:
  `git tag -a "v1.4" -m "1.4 release"`
* Push the tag to Github: `git push --tags origin`
* Run `goreleaser --rm-dist`

The configuration for GoReleaser can be found in `.goreleaser.yml`.
