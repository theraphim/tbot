package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/yanzay/tbot/v2"
)

type application struct {
	client  *tbot.Client
	votings map[string]*voting
}

type voting struct {
	ups   int
	downs int
}

func main() {
	token := os.Getenv("TELEGRAM_TOKEN")
	bot := tbot.New(token)
	app := &application{
		votings: make(map[string]*voting),
	}
	app.client = bot.Client()
	bot.HandleMessage("/vote", app.votingHandler)
	bot.HandleCallback(app.callbackHandler)
	bot.Start()
}

func (a *application) votingHandler(m *tbot.Message) {
	buttons := makeButtons(0, 0)
	msg, _ := a.client.SendMessage(tbot.ChatID(m.Chat.ID), "Please vote", tbot.OptInlineKeyboardMarkup(buttons))
	votingID := strconv.FormatInt(m.Chat.ID, 10) + ":" + strconv.Itoa(msg.MessageID)
	a.votings[votingID] = &voting{}
}

func (a *application) callbackHandler(cq *tbot.CallbackQuery) {
	votingID := strconv.FormatInt(cq.Message.Chat.ID, 10) + ":" + strconv.Itoa(cq.Message.MessageID)
	v := a.votings[votingID]
	if cq.Data == "up" {
		v.ups++
	}
	if cq.Data == "down" {
		v.downs++
	}
	buttons := makeButtons(v.ups, v.downs)
	a.client.EditMessageReplyMarkup(tbot.ChatID(cq.Message.Chat.ID), cq.Message.MessageID, tbot.OptInlineKeyboardMarkup(buttons))
	a.client.AnswerCallbackQuery(cq.ID, tbot.OptText("OK"))
}

func makeButtons(ups, downs int) *tbot.InlineKeyboardMarkup {
	button1 := tbot.InlineKeyboardButton{
		Text:         fmt.Sprintf("👍 %d", ups),
		CallbackData: "up",
	}
	button2 := tbot.InlineKeyboardButton{
		Text:         fmt.Sprintf("👎 %d", downs),
		CallbackData: "down",
	}
	return &tbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]tbot.InlineKeyboardButton{
			[]tbot.InlineKeyboardButton{button1, button2},
		},
	}
}
