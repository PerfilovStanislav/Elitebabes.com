package main

import (
	"Elitebabes.com/elite_model"
	"Elitebabes.com/shared"
	"fmt"
	tgbotapi "github.com/PerfilovStanislav/telegram-bot-api"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"net/http"
	"os"
	"strconv"
	"strings"
)

var (
	debugChannelId int64
	likeBonusBot   *shared.Bot
)

func main() {
	shared.SingleProcess("LikeBonusBot")
	shared.LoadEnv()
	var db = shared.ConnectToDb()

	debugChannelId, _ = strconv.ParseInt(os.Getenv("CHANNEL_FOR_TEST_ID"), 10, 64)
	likeBonusBot = shared.NewBot(os.Getenv("LIKE_BONUS_BOT_TOKEN"), debugChannelId)

	_, _ = likeBonusBot.SetWebhook(tgbotapi.NewWebhook("https://richinme.com/go/elitebabes/bonus_bot/" + likeBonusBot.Token))
	updates := likeBonusBot.ListenForWebhook("/go/elitebabes/bonus_bot/" + likeBonusBot.Token)
	go http.ListenAndServe(":8003", nil)

	for update := range updates {
		if update.Message != nil {
			var message = update.Message
			var texts = strings.Split(update.Message.Text, " ")
			if texts[0] == "/showbonuses" {
				showBonuses(db, message)
			} else if texts[0] == "/invitelink" {
				inviteLink(message)
			} else if texts[0] == "/start" {
				if len(texts) == 1 {
					showBonuses(db, message)
				} else if len(texts) == 2 {
					var fromFriendId, _ = strconv.Atoi(texts[1])
					var referralBonus = getBonus(db, message.From.ID)
					if referralBonus.FromId == 0 { // Ещё не зареган
						insertReferral(db, fromFriendId, message.From.ID)
						welcome(message)
						shared.AddBonus(db, message.From.ID, 50.0, 4)
					}
				}
			}
		}
	}
}

func insertReferral(db *sqlx.DB, fromFriendId, userId int) {
	_, _ = db.Exec(`INSERT INTO referrals (parent_id, user_id) VALUES ($1, $2)`,
		fromFriendId, userId)
}

func welcome(message *tgbotapi.Message) {
	var config = tgbotapi.NewMessage(message.Chat.ID,
		"Поздравляю! Ты получил приглашение и заработал свои первые *бонусы* 💰 "+
			"Воспользуйся командой для генерации своей ссылки и начни приглашать друзей!")
	config.ParseMode = tgbotapi.ModeMarkdown
	likeBonusBot.ReSend(config)
}

func inviteLink(message *tgbotapi.Message) {
	var config = tgbotapi.NewMessage(message.Chat.ID,
		fmt.Sprintf("Приглашаю тебя зарабатывать бонусы вместе c [BonusBot](tg://resolve?domain=%s&start=%d)",
			os.Getenv("LIKE_BONUS_BOT_NAME"), message.From.ID),
	)
	config.ParseMode = tgbotapi.ModeMarkdown
	likeBonusBot.ReSend(config)
}

func showBonuses(db *sqlx.DB, message *tgbotapi.Message) {
	var bonus = getBonus(db, message.From.ID)
	var config = tgbotapi.NewMessage(message.Chat.ID,
		fmt.Sprintf("У вас *%d* бонус%s", int(bonus.Bonus), shared.PluralPostfix(int(bonus.Bonus))))
	config.ParseMode = tgbotapi.ModeMarkdown
	likeBonusBot.ReSend(config)
}

func getBonus(db *sqlx.DB, fromId int) elite_model.Bonus {
	var bonus elite_model.Bonus
	_ = db.Get(&bonus, "SELECT from_id, bonus FROM bonuses where from_id = $1", fromId)
	return bonus
}
