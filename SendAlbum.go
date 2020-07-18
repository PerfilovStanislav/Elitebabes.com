package main

import (
	"Elitebabes.com/elite_model"
	"Elitebabes.com/shared"
	"encoding/json"
	"fmt"
	tgbotapi "github.com/PerfilovStanislav/telegram-bot-api"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const SentSecondPartId = 7
const SentLastPartId = 8

var (
	testChannelId int64
	//likeBonusBot  *shared.Bot
	sendPhotosBot *shared.Bot
)

func main() {
	shared.SingleProcess("ParsePhotos")
	shared.LoadEnv()
	var db = shared.ConnectToDb()

	testChannelId, _ = strconv.ParseInt(os.Getenv("CHANNEL_FOR_TEST_ID"), 10, 64)
	//likeBonusBot = shared.NewBot(os.Getenv("LIKE_BONUS_BOT"), testChannelId)
	sendPhotosBot = shared.NewBot(os.Getenv("SEND_PHOTOS_BOT_TOKEN"), testChannelId)

	//sendPhotosBot.SetWebhook(tgbotapi.NewWebhook("https://3be2cd0e8b71.ngrok.io/" + sendPhotosBot.Token))
	updates := sendPhotosBot.ListenForWebhook("/" + sendPhotosBot.Token)
	go http.ListenAndServe(":3001", nil)

	for update := range updates {
		if update.CallbackQuery != nil {
			var callback = update.CallbackQuery
			var _, err = getLike(db, callback)
			if err == nil {
				answer(callback.ID, "Вы уже голосовали ☺️")
			} else {
				var payload Payload
				_ = json.Unmarshal([]byte(callback.Data), &payload)

				var messageId = callback.Message.MessageID
				var likes, dislikes = getCountOfLikes(db, callback.Message.Chat.ID, messageId)
				var bonus = int(math.Max(math.Round(32.0/math.Pow(likes+dislikes+1.0, 0.3)-12.0), 1.0))
				insertLike(db, payload.Value, callback)
				if payload.Value {
					likes++
				} else {
					dislikes++
				}
				addBonus(db, callback, bonus)

				sendPhotosBot.ReSend(
					tgbotapi.NewEditMessageReplyMarkup(
						callback.Message.Chat.ID,
						messageId,
						shared.ReplyMarkupLikes(payload.LinkId, int(likes), int(dislikes)),
					),
				)
				answer(callback.ID, fmt.Sprintf("Вы получили %d бонус%s 💰", bonus, pluralPostfix(bonus)))

				var link = getLink(db, payload.LinkId)
				if link.Status == SentLastPartId {
					continue
				}

				var countOfPhotos = getCountOfPhotos(db, payload.LinkId)
				if countOfPhotos >= 3 && countOfPhotos <= 6 {
					if link.Status < SentLastPartId {
						if quiteliked(float64(countOfPhotos)-1.0, likes, dislikes) {
							// отправить всю партию
							sendPhotos(db, messageId, 2, countOfPhotos, SentLastPartId, link)
						}
					}
				} else {
					if countOfPhotos >= 7 && countOfPhotos <= 8 {
						if link.Status < SentSecondPartId {
							if quiteliked(3.0, likes, dislikes) {
								// отправить первую партию
								sendPhotos(db, messageId, 2, 4, SentSecondPartId, link)
							}
						}
					} else if countOfPhotos >= 9 {
						if link.Status < SentSecondPartId {
							if quiteliked(4.0, likes, dislikes) {
								// отправить первую партию
								sendPhotos(db, messageId, 2, 5, SentSecondPartId, link)
							}
						}
					}
					if quiteliked(float64(countOfPhotos)-1.0, likes, dislikes) {
						// отправить вторую партию
						if link.Status < SentLastPartId {
							if countOfPhotos >= 7 && countOfPhotos <= 8 {
								sendPhotos(db, messageId, 5, countOfPhotos, SentLastPartId, link)
							} else {
								sendPhotos(db, messageId, 6, countOfPhotos, SentLastPartId, link)
							}
						}
					}
				}
			}
		}
	}
}

func getLink(db *sqlx.DB, linkId int) elite_model.Link {
	link := elite_model.Link{}
	_ = db.Get(&link, "SELECT id, model, status FROM links WHERE id=$1", linkId)
	return link
}

func sendPhotos(db *sqlx.DB, messageId, fromRow, toRow, partId int, link elite_model.Link) {
	var medias []elite_model.Media
	_ = db.Select(&medias, "SELECT file_id FROM media where link_id = $1 and row >= $2 and row <= $3 order by row",
		link.Id, fromRow, toRow)
	var files []interface{}
	for i, media := range medias {
		inpMedia := tgbotapi.NewInputMediaPhoto(media.FileId)
		if i == 0 {
			inpMedia.ParseMode = tgbotapi.ModeMarkdown
			inpMedia.Caption = fmt.Sprintf("[Channel](tg://resolve?domain=%s) #Album #%s",
				os.Getenv("CHANNEL_FOR_TEST_NAME"), strings.Replace(link.Model, " ", "", -1))
		}
		files = append(files, inpMedia)
	}

	config := tgbotapi.NewMediaGroup(testChannelId, files)
	config.BaseChat.ReplyToMessageID = messageId
	config.BaseChat.DisableNotification = true
	sendPhotosBot.ReSendGroup(config)

	_, _ = db.Exec(`UPDATE links SET status = $1 where id = $2`, partId, link.Id)
}

func quiteliked(countOfPhotos float64, likes float64, dislikes float64) bool {
	if dislikes == 0 {
		dislikes = countOfPhotos - likes
	}
	return likes/(likes+dislikes) >= 0.7
}

func getCountOfPhotos(db *sqlx.DB, linkId int) int {
	var count int
	_ = db.QueryRowx("SELECT count(*) from media where link_id=$1 and row <= 15 and row != 0", linkId).Scan(&count)
	return count
}

type Payload struct {
	LinkId int  `json:"link_id"`
	Value  bool `json:"value"`
}

func insertLike(db *sqlx.DB, like bool, callback *tgbotapi.CallbackQuery) {
	var _, err = db.Exec("INSERT INTO likes (chat_id, from_id, message_id, is_liked) VALUES ($1, $2, $3, $4)",
		callback.Message.Chat.ID,
		callback.From.ID,
		callback.Message.MessageID,
		like)
	if err != nil {
		fmt.Println(err)
	}
}

func pluralPostfix(count int) string {
	switch {
	case 5 <= count && count <= 20:
		return "ов"
	case count%10 == 1:
		return ""
	case count%10 <= 4:
		return "а"
	default:
		return "ов"
	}
}

func addBonus(db *sqlx.DB, callback *tgbotapi.CallbackQuery, bonus int) {
	_, _ = db.Exec("INSERT INTO bonuses (from_id, bonus) "+
		"VALUES ($1, $2) "+
		"ON CONFLICT (from_id) DO UPDATE "+
		"SET bonus = bonuses.bonus + excluded.bonus",
		callback.From.ID, bonus)
}

func getCountOfLikes(db *sqlx.DB, chatId int64, messageId int) (float64, float64) {
	var likes, dislikes float64
	_ = db.QueryRowx("SELECT count(nullif(is_liked, false)) likes, count(nullif(is_liked, true)) dislikes "+
		"from likes where chat_id=$1 and message_id=$2",
		chatId, messageId).Scan(&likes, &dislikes)
	return likes, dislikes
}

func answer(callbackId string, message string) {
	config := tgbotapi.CallbackConfig{
		CallbackQueryID: callbackId,
		Text:            message,
		ShowAlert:       false,
		CacheTime:       0,
	}
	_, _ = sendPhotosBot.AnswerCallbackQuery(config)
}

func getLike(db *sqlx.DB, callback *tgbotapi.CallbackQuery) (elite_model.Like, error) {
	like := elite_model.Like{}
	return like, db.Get(&like, "SELECT id, chat_id, from_id, message_id, is_liked "+
		"FROM likes WHERE chat_id=$1 and from_id=$2 and message_id=$3",
		callback.Message.Chat.ID,
		callback.From.ID,
		callback.Message.MessageID)
}
