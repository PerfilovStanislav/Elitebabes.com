package main

import (
	"Elitebabes.com/elite_model"
	"Elitebabes.com/shared"
	"fmt"
	tgbotapi "github.com/PerfilovStanislav/telegram-bot-api"
	_ "github.com/lib/pq"
	"os"
	"strconv"
	"strings"
)

var (
	sendPhotoChannelId int64
)

const SentFirstPhotoId = 6

func main() {
	shared.LoadEnv()
	var db = shared.ConnectToDb()

	sendPhotosBot := shared.NewBot(os.Getenv("SEND_PHOTOS_BOT_TOKEN"), 0)
	sendPhotoChannelId, _ = strconv.ParseInt(os.Getenv("CHANNEL_FOR_TEST_ID"), 10, 64)

	link := elite_model.Link{}
	err := db.Get(&link, "SELECT id, model, description FROM links WHERE status=1 order by random() limit 1")
	if err != nil {
		return
	}

	photo := elite_model.Media{}
	err = db.Get(&photo, "SELECT id, link_id, file_id FROM media WHERE link_id=$1 and row=1 LIMIT 1", link.Id)
	if err != nil {
		return
	}

	var photoCount int
	_ = db.QueryRowx(`SELECT count(*) FROM media WHERE link_id=$1 and row <= 15 and row != 0`, link.Id).Scan(&photoCount)

	msg := tgbotapi.NewPhotoShare(sendPhotoChannelId, photo.FileId)
	msg.ReplyMarkup = shared.ReplyMarkupLikes(link.Id, 0, 0)
	msg.BaseChat.DisableNotification = true
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.Caption = link.Description
	msg.Caption = fmt.Sprintf("*Модель:* #%s\n*Фотографий:* %d\n\n%s\n\n[Channel](tg://resolve?domain=%s) #Preview",
		strings.Replace(link.Model, " ", "", -1),
		photoCount, link.Description, os.Getenv("CHANNEL_FOR_TEST_NAME"))

	sendPhotosBot.ReSend(msg)

	_, _ = db.Exec(`UPDATE links SET status = $1 where id = $2`, SentFirstPhotoId, link.Id)
}
