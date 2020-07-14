package shared

import (
	"fmt"
	tgbotapi "github.com/PerfilovStanislav/telegram-bot-api"
	"time"
)

type Bot struct {
	*tgbotapi.BotAPI
	ChannelId int64
}

func NewBot(token string, channelId int64) *Bot {
	bot, _ := tgbotapi.NewBotAPI(token)
	return &Bot{
		BotAPI:    bot,
		ChannelId: channelId,
	}
}

func (b *Bot) ReSend(c tgbotapi.Chattable) tgbotapi.Message {
	var resp, err = b.BotAPI.Send(c)
	if err != nil {
		var botError = err.(*tgbotapi.Error)
		if botError.RetryAfter > 0 {
			time.Sleep(time.Second * (time.Duration(botError.RetryAfter) + 1))
			return b.ReSend(c)
		} else if botError.Code == 400 {
			var respRetry, err2 = b.Send(c)
			if err2 != nil {
				b.BotAPI.Send(tgbotapi.NewMessage(b.ChannelId, botError.Message))
			}
			return respRetry
		} else {
			fmt.Println(resp, err)
		}
	}
	return resp
}

func (b *Bot) ReSendGroup(c tgbotapi.Chattable) []tgbotapi.Message {
	var resp, err = b.BotAPI.SendGroup(c)
	if err != nil {
		var botError = err.(*tgbotapi.Error)
		if botError.RetryAfter > 0 {
			time.Sleep(time.Second * (time.Duration(botError.RetryAfter) + 1))
			return b.ReSendGroup(c)
		} else if botError.Code == 400 {
			time.Sleep(time.Second * time.Duration(1))
			var respRetry, err2 = b.BotAPI.SendGroup(c)
			if err2 != nil {
				b.BotAPI.Send(tgbotapi.NewMessage(b.ChannelId, botError.Message))
			}
			return respRetry
		} else {
			fmt.Println(resp, err)
		}
	}
	return resp
}
