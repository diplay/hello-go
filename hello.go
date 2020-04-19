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
	"regexp"
	"strconv"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

const baseDir = "/tmp/ytdl/"
const commandName = "youtube-dl"
const staticPrefix = "/static/"

var destinationRegex = regexp.MustCompile("\\[ffmpeg\\] Destination: (?:.*\\/(.+)|(.+))")

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

func doDownload(id string) (string, string, error) {
	infoFilename := baseDir + id + ".info.json"
	if _, err := os.Stat(infoFilename); os.IsNotExist(err) {
		filename := "'" + baseDir + "%(id)s.%(ext)s'"
		commandParams := "-x --write-info-json --no-progress -f 'worstaudio/worst' -o " + filename + " -- " + id
		command := commandName + " " + commandParams
		cmd := exec.Command("sh", "-c", command)

		log.Printf("Run %s\n", command)
		out, err := cmd.Output()

		log.Printf("The %s output is\n%s\n", command, out)

		var resultFilename string
		for _, match := range destinationRegex.FindSubmatch(out) {
			if len(match) > 0 {
				resultFilename = string(match)
			}
		}

		return resultFilename, command, err
	}
	return "", "", nil
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

	filename, command, err := doDownload(id)
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

	audioFilename := staticPrefix + filename
	audioURL := audioFilename + "#t=" + t
	data := listenTemplateData{Title: filename, AudioURL: audioURL, AudioFile: audioFilename, Time: time}

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

func processTelegramUpdates(bot *tgbotapi.BotAPI, listenBaseURL string, updates tgbotapi.UpdatesChannel) {
	for update := range updates {
		// log.Printf("Update from telegram %+v\nMessage: %+v\n", update, update.Message)
		if update.Message == nil {
			continue
		}

		switch update.Message.Command() {
		case "start":
			log.Printf("Received /start command from %s\n", update.Message.From.UserName)
			sendTextMessage(bot, update.Message, "Use this bot to get an audio from youtube videos")
		case "info":
			log.Printf("Received /info command from %s\n", update.Message.From.UserName)
			sendTextMessage(bot, update.Message, "Use this bot to get an audio from youtube videos")
		case "listen":
			args := update.Message.CommandArguments()
			log.Printf("Received /listen command with video %s from %s\n", args, update.Message.From.UserName)

			id := extractVideoID(args)

			if len(id) == 0 {
				sendTextMessage(bot, update.Message, "Parameter 'v' is invalid. Must be an url like 'https://youtu.be/b8g1o8Ph7LQ' or 'https://www.youtube.com/watch?v=b8g1o8Ph7LQ' or just 'b8g1o8Ph7LQ'.")
				continue
			}

			log.Printf("Received listen command, video id %s", id)
			if _, loaded := idsInProgress.LoadOrStore(id, 1); loaded {
				log.Printf("Cannot set id %s to active state", id)
				sendTextMessage(bot, update.Message, "Other request is downloading video "+id+" now, please try later")
				continue
			}

			sendTextMessage(bot, update.Message, "Wait a moment, downloading the content for you")
			bot.Send(tgbotapi.NewChatAction(update.Message.Chat.ID, "typing"))
			filename, command, err := doDownload(id)
			idsInProgress.Delete(id)

			if err != nil {
				sendTextMessage(bot, update.Message, "Command "+command+" error: "+err.Error())
				log.Printf("Command %s error: %s", command, err.Error())
				return
			}

			sendTextMessage(bot, update.Message, listenBaseURL+"?v="+filename+"&t=0")
		default:
			log.Printf("Cannot parse message %+v", update.Message)
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
