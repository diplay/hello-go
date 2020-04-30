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

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

const baseDir = "/tmp/ytdl/"
const commandName = "youtube-dl"
const staticPrefix = "/static/"

const parameterVInvalidMessage = "Parameter 'v' is invalid. Must be an url like 'https://youtu.be/b8g1o8Ph7LQ' or 'https://www.youtube.com/watch?v=b8g1o8Ph7LQ' or just 'b8g1o8Ph7LQ'."
const info = `Use this bot to get an audio from youtube videos.
Examples:
- /listen https://youtu.be/b8g1o8Ph7LQ
- /listen b8g1o8Ph7LQ
- /listen b8g1o8Ph7LQ mp3
- https://youtu.be/b8g1o8Ph7LQ
- https://www.youtube.com/watch?v=b8g1o8Ph7LQ
etc...
`

var idsInProgress sync.Map
var listenTemplate = template.Must(template.ParseFiles("listen.html"))

type listenTemplateData struct {
	Title     string
	AudioFile string
	AudioURL  string
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

func findOutputFile(id string) string {
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

func doDownload(id, audioFormat string) (string, string, error) {
	var fileToDeleteIfDownloadSucceed string
	if resultFilename := findOutputFile(id); len(resultFilename) > 0 {
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

	if resultFilename := findOutputFile(id); len(resultFilename) > 0 {
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

	filename, command, err := doDownload(id, audioFormat)
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

func sendTextMessage(bot *tgbotapi.BotAPI, answerTo *tgbotapi.Message, text string) {
	msg := tgbotapi.NewMessage(answerTo.Chat.ID, text)
	msg.ReplyToMessageID = answerTo.MessageID
	msg.DisableWebPagePreview = true
	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("Cannot send a message to telegram: %s", err.Error())
	}
}

func telegramListenHandle(bot *tgbotapi.BotAPI, commandMessage *tgbotapi.Message, listenBaseURL, id, audioFormat string) {
	id = extractVideoID(id)
	if len(id) == 0 {
		sendTextMessage(bot, commandMessage, parameterVInvalidMessage)
		return
	}

	log.Printf("Received listen command, video id %s", id)
	if _, loaded := idsInProgress.LoadOrStore(id, 1); loaded {
		log.Printf("Cannot set id %s to active state", id)
		sendTextMessage(bot, commandMessage, "Other request is downloading video "+id+" now, please try later")
		return
	}

	sendTextMessage(bot, commandMessage, "Wait a moment, downloading the content for you")
	bot.Send(tgbotapi.NewChatAction(commandMessage.Chat.ID, "typing"))
	filename, command, err := doDownload(id, audioFormat)
	idsInProgress.Delete(id)

	if err != nil {
		sendTextMessage(bot, commandMessage, "Cannot load a video with id "+id)
		log.Printf("Command %s error: %s", command, err.Error())
		return
	}

	sendTextMessage(bot, commandMessage, listenBaseURL+"?v="+filename+"&t=0")
}

func parseArgs(args string) (string, string) {
	splitted := strings.Split(args, " ")
	if len(splitted) == 0 {
		return "", ""
	} else if len(splitted) == 1 {
		return splitted[0], ""
	} else {
		return splitted[0], splitted[1]
	}
}

func processTelegramUpdates(bot *tgbotapi.BotAPI, listenBaseURL string, updates tgbotapi.UpdatesChannel) {
	for update := range updates {
		// log.Printf("Update from telegram %+v\nMessage: %+v\n", update, update.Message)
		if update.Message == nil {
			continue
		}

		switch update.Message.Command() {
		case "start":
			log.Printf("Received /start command from %s\n", update.Message.From.UserName)
			sendTextMessage(bot, update.Message, info)
		case "info":
			log.Printf("Received /info command from %s\n", update.Message.From.UserName)
			sendTextMessage(bot, update.Message, info)
		case "listen":
			args := update.Message.CommandArguments()
			log.Printf("Received /listen command with video %s from %s\n", args, update.Message.From.UserName)
			id, audioFormat := parseArgs(args)
			telegramListenHandle(bot, update.Message, listenBaseURL, id, audioFormat)
		default:
			id, audioFormat := parseArgs(update.Message.Text)
			telegramListenHandle(bot, update.Message, listenBaseURL, id, audioFormat)
		}
	}
}

func initBotAPI(domain, token string) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal(err)
	}
	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	_, err = bot.SetWebhook(tgbotapi.NewWebhook("https://" + domain + "/" + bot.Token))
	if err != nil {
		log.Fatal(err)
	}
	info, err := bot.GetWebhookInfo()
	if err != nil {
		log.Fatal(err)
	}
	if info.LastErrorDate != 0 {
		log.Printf("Telegram callback failed: %s", info.LastErrorMessage)
	}

	updates := bot.ListenForWebhook("/" + bot.Token)
	go processTelegramUpdates(bot, "https://"+domain+"/listen", updates)
}

func main() {
	useHTTPS := flag.Bool("https", false, "Use HTTPS")
	domain := flag.String("domain", "localhost", "Domain for telegram webhook")
	addr := flag.String("addr", "127.0.0.1:8080", "Address to listen for HTTP protocol")
	cert := flag.String("cert", "localhost.crt", "Certificate file for HTTPS")
	key := flag.String("key", "localhost.key", "Key file for HTTPS")
	telegramBotToken := flag.String("telegram-bot-token", "", "Token for telegram bot")
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	http.HandleFunc("/download", downloadHandle)
	http.HandleFunc("/listen", listenHandle)
	http.Handle(staticPrefix, http.StripPrefix(staticPrefix, http.FileServer(http.Dir("/tmp/ytdl/"))))

	if len(*telegramBotToken) > 0 {
		initBotAPI(*domain, *telegramBotToken)
	} else {
		log.Println("Don't create a webhook for telegram bot")
	}

	var err error

	if *useHTTPS {
		log.Println("Using HTTPS")
		err = http.ListenAndServeTLS(":443", *cert, *key, nil)
	} else {
		log.Println("Using HTTP with address", *addr)
		err = http.ListenAndServe(*addr, nil)
	}

	if err != nil {
		log.Println("Error", err.Error())
	}
}
