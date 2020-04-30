package internal

import (
	"log"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var idsInProgress sync.Map

// ExtractVideoID TODO
func ExtractVideoID(v string) string {
	if youtubeURL, err := url.ParseRequestURI(v); err == nil {
		if id := youtubeURL.Query().Get("v"); len(id) > 0 {
			return id
		}

		splitted := strings.Split(youtubeURL.Path, "/")
		id := splitted[len(splitted)-1]
		return id
	}

	// if v is not a valid URL try some heuristics
	if strings.Contains(v, "v=") {
		splitted := strings.Split(v, "v=")
		v = splitted[len(splitted)-1]
	}
	if strings.Contains(v, "/") {
		splitted := strings.Split(v, "/")
		v = splitted[len(splitted)-1]
	}

	return v
}

// FindOutputFile TODO
func FindOutputFile(id string) string {
	if dir, err := os.Open(baseDir); err == nil {
		if files, err := dir.Readdirnames(-1); err == nil {
			for _, filename := range files {
				if strings.HasPrefix(filename, id) && !strings.HasSuffix(filename, ".info.json") {
					return filename
				}
			}
		}
	}
	return ""
}

// DoDownload TODO
func DoDownload(id, audioFormat string) (string, string, error) {
	var fileToDeleteIfDownloadSucceed string
	if resultFilename := FindOutputFile(id); len(resultFilename) > 0 {
		log.Printf("Found existing file %s\n", resultFilename)
		if len(audioFormat) == 0 || (len(audioFormat) > 0 && strings.HasSuffix(resultFilename, audioFormat)) {
			return resultFilename, "", nil
		}

		log.Printf("Existing file %s does not match wanted type %s\n", resultFilename, audioFormat)
		fileToDeleteIfDownloadSucceed = baseDir + resultFilename
	}

	filename := "'" + baseDir + "%(id)s.%(ext)s'"
	commandParams := "-x --write-info-json --no-progress -f 'bestaudio[filesize<20M]/best[filesize<20M]/worstaudio/worst' -o " + filename
	if len(audioFormat) > 0 {
		commandParams += " --audio-format '" + audioFormat + "'"
	}
	command := commandName + " " + commandParams + " -- " + id
	cmd := exec.Command("sh", "-c", command)

	log.Printf("Run %s\n", command)
	out, err := cmd.Output()
	log.Printf("The %s output is\n%s\n", command, out)

	if resultFilename := FindOutputFile(id); len(resultFilename) > 0 {
		if len(fileToDeleteIfDownloadSucceed) > 0 {
			err := os.RemoveAll(fileToDeleteIfDownloadSucceed)
			if err != nil {
				log.Printf("Cannot delete %s", resultFilename)
			}
		}
		return resultFilename, "", err
	}

	log.Printf("Cannot find output file for video %s", id)
	return "", "", err
}
