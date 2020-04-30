package internal

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
)

var listenTemplate = template.Must(template.ParseFiles("web/listen.html"))

type listenTemplateData struct {
	Title     string
	AudioFile string
	AudioURL  string
	Time      int64
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
	id = ExtractVideoID(id)

	if len(id) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, parameterVInvalidMessage)
		return
	}

	log.Printf("Received request %s, video id %s", r.URL.String(), id)
	if _, loaded := idsInProgress.LoadOrStore(id, 1); loaded {
		log.Printf("Cannot set id %s to active state", id)
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, "Other request is downloading video %s now, please try later", id)
		return
	}

	convertToMP3 := r.URL.Query().Get("mp3") == "on"
	log.Printf("Received '%s' flag for convert to mp3\n", convertToMP3)
	audioFormat := ""
	if convertToMP3 {
		audioFormat = "mp3"
	}

	filename, command, err := DoDownload(id, audioFormat)
	idsInProgress.Delete(id)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Command %s error: %s", command, err.Error())
		log.Printf("Command %s error: %s", command, err.Error())
		return
	}

	http.Redirect(w, r, "/listen?v="+filename+"&t=0", http.StatusFound)
}

func listenHandle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "Make a GET request")
		return
	}

	filename := r.URL.Query().Get("v")
	if len(filename) == 0 {
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

	audioURL := staticPrefix + filename + "#t=" + t
	data := listenTemplateData{Title: filename, AudioURL: audioURL, AudioFile: filename, Time: time}

	w.Header().Add("Feature-Policy", "autoplay 'self'")

	err = listenTemplate.Execute(w, data)
	if err != nil {
		fmt.Fprintf(w, "Error: %s", err.Error())
		log.Printf("Error: %s", err.Error())
	}
}

// RunHTTPServer TODO
func RunHTTPServer (useHTTPS bool, cert, key, addr string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/index.html")
	})
	http.HandleFunc("/download", downloadHandle)
	http.HandleFunc("/listen", listenHandle)
	http.Handle(staticPrefix, http.StripPrefix(staticPrefix, http.FileServer(http.Dir("/tmp/ytdl/"))))

	var err error

	if useHTTPS {
		log.Println("Using HTTPS")
		err = http.ListenAndServeTLS(":443", cert, key, nil)
	} else {
		log.Println("Using HTTP with address", addr)
		err = http.ListenAndServe(addr, nil)
	}

	if err != nil {
		log.Println("Error", err.Error())
	}
}
