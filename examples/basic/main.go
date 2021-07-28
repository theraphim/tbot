package main

import (
	"log"
	"os"
	"time"

	"github.com/yanzay/tbot/v2"
)

func main() {
	bot := tbot.New(os.Getenv("TELEGRAM_TOKEN"))
	c := bot.Client()
	bot.HandleMessage(".*yo.*", func(m *tbot.Message) {
		c.SendChatAction(tbot.ChatID(m.Chat.ID), tbot.ActionTyping)
		time.Sleep(1 * time.Second)
		c.SendMessage(tbot.ChatID(m.Chat.ID), "hello!")
	})
	err := bot.Start()
	if err != nil {
		log.Fatal(err)
	}
}
