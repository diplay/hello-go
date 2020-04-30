
package main

import (
	"flag"
	"log"
	"hello-go/internal"
)

func main() {
	useHTTPS := flag.Bool("https", false, "Use HTTPS")
	domain := flag.String("domain", "localhost", "Domain for telegram webhook")
	addr := flag.String("addr", "127.0.0.1:8080", "Address to listen for HTTP protocol")
	cert := flag.String("cert", "localhost.crt", "Certificate file for HTTPS")
	key := flag.String("key", "localhost.key", "Key file for HTTPS")
	telegramBotToken := flag.String("telegram-bot-token", "", "Token for telegram bot")
	flag.Parse()

	if len(*telegramBotToken) > 0 {
		internal.InitBotAPI(*domain, *telegramBotToken)
	} else {
		log.Println("Don't create a webhook for telegram bot")
	}

	internal.RunHTTPServer(*useHTTPS, *cert, *key, *addr)
}