package internal

import (
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

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

// InitBotAPI TODO
func InitBotAPI(domain, token string) {
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
