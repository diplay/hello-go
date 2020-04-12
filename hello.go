package main

import (
    "fmt"
    "net/http"
    "os"
    "os/exec"
)

const audioFormat = "mp3"
const extension = "." + audioFormat
const baseDir = "/tmp/ytdl/"
const commandName = "youtube-dl"

func download_handle(w http.ResponseWriter, r *http.Request) {
    if (r.Method != "GET") {
        w.WriteHeader(http.StatusMethodNotAllowed)
        fmt.Fprintf(w, "Make a GET request")
        return
    }

    id := r.URL.Query().Get("v")
    if (len(id) == 0) {
        w.WriteHeader(http.StatusBadRequest)
        fmt.Fprintf(w, "Must specify 'v' parameter")
        return
    }

    resultFilename := baseDir + id + extension

    if _, err := os.Stat(resultFilename); os.IsNotExist(err) {
        filename := "'" + baseDir + id + ".webm'"
        commandParams := "-x --audio-format '" + audioFormat + "' -o " + filename + " -- " + id
        command := commandName + " " + commandParams
        cmd := exec.Command("bash", "-c", command)
        err := cmd.Run()

        if (err != nil) {
            w.WriteHeader(http.StatusInternalServerError)
            fmt.Fprintf(w, "Command " + command + " error: " + err.Error())
        }
    }

    http.Redirect(w, r, "/static/" + id + extension, http.StatusFound)
}

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, "index.html")
    })
    http.HandleFunc("/download", download_handle)
    http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("/tmp/ytdl/"))))

    http.ListenAndServe("127.0.0.1:8080", nil)
}
