package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/acme/autocert"
)

const audioFormat = "mp3"
const extension = "." + audioFormat
const baseDir = "/tmp/ytdl/"
const commandName = "youtube-dl"
const staticPrefix = "/static/"

var idsInProgress sync.Map
var listenTemplate = template.Must(template.ParseFiles("listen.html"))

type listenTemplateData struct {
	ID        string
	AudioFile string
	Time      int64
}

func extractVideoID(v string) string {
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

func doDownload(id string) (string, error) {
	resultFilename := baseDir + id + extension
	if _, err := os.Stat(resultFilename); os.IsNotExist(err) {
		filename := "'" + baseDir + id + ".webm'"
		commandParams := "-x --audio-format '" + audioFormat + "' -o " + filename + " -- " + id
		command := commandName + " " + commandParams
		cmd := exec.Command("sh", "-c", command)

		log.Printf("Run %s\n", command)
		out, err := cmd.Output()

		log.Printf("The %s output is\n%s\n", command, out)

		return command, err
	}
	return "", nil
}

func downloadHandle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "Make a GET request")
		return
	}

	id := r.URL.Query().Get("v")
	if len(id) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Must specify 'v' parameter")
		return
	}
	id = extractVideoID(id)

	if len(id) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(
			w,
			"Parameter 'v' is invalid. Must be an url like 'https://youtu.be/b8g1o8Ph7LQ' or 'https://www.youtube.com/watch?v=b8g1o8Ph7LQ' or just 'b8g1o8Ph7LQ'.",
		)
		return
	}

	log.Printf("Received request %s, video id %s", r.URL.String(), id)
	if _, loaded := idsInProgress.LoadOrStore(id, 1); loaded {
		log.Printf("Cannot set id %s to active state", id)
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, "Other request is downloading video %s now, please try later", id)
		return
	}

	command, err := doDownload(id)
	idsInProgress.Delete(id)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Command %s error: %s", command, err.Error())
		log.Printf("Command %s error: %s", command, err.Error())
		return
	}

	http.Redirect(w, r, "/listen?v="+id+"&t=0", http.StatusFound)
}

func listenHandle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "Make a GET request")
		return
	}

	id := r.URL.Query().Get("v")
	if len(id) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Must specify 'v' parameter")
		return
	}

	t := r.URL.Query().Get("t")
	if len(t) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Must specify 't' parameter")
		return
	}

	time, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Parameter 't' must be integer")
		return
	}

	audioFilename := staticPrefix + id + extension + "#t=" + t
	data := listenTemplateData{ID: id, AudioFile: audioFilename, Time: time}

	w.Header().Add("Feature-Policy", "autoplay 'self'")

	err = listenTemplate.Execute(w, data)
	if err != nil {
		fmt.Fprintf(w, "Error: %s", err.Error())
		log.Printf("Error: %s", err.Error())
	}
}

func main() {
	useHTTPS := flag.Bool("https", false, "Use HTTPS")
	domain := flag.String("domain", "localhost", "Domain for HTTPS certificate")
	addr := flag.String("addr", "127.0.0.1:8080", "Address to listen for HTTP protocol")
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	http.HandleFunc("/download", downloadHandle)
	http.HandleFunc("/listen", listenHandle)
	http.Handle(staticPrefix, http.StripPrefix(staticPrefix, http.FileServer(http.Dir("/tmp/ytdl/"))))

	var err error

	if *useHTTPS {
		log.Println("Using HTTPS")
		if *domain == "localhost" {
			err = http.ListenAndServeTLS(":443", "localhost.crt", "localhost.key", nil)
		} else {
			err = http.Serve(autocert.NewListener(*domain), nil)
		}
	} else {
		log.Println("Using HTTP with address", *addr)
		err = http.ListenAndServe(*addr, nil)
	}

	if err != nil {
		log.Println("Error", err.Error())
	}
}
