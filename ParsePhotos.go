package main

import (
	"Elitebabes.com/elite_model"
	"Elitebabes.com/shared"
	"encoding/json"
	"fmt"
	tgbotapi "github.com/PerfilovStanislav/telegram-bot-api"
	"github.com/antchfx/htmlquery"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"golang.org/x/net/html"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const ActionPublicId = 1
const ActionDeleteId = 2
const ActionToggleId = 3
const ActionNameId = 4
const ActionDescriptionId = 5

var (
	parseChannelId int64
	sendPhotosBot  *tgbotapi.BotAPI
	parseSiteBot   *tgbotapi.BotAPI
)

func main() {
	shared.SingleProcess("ParsePhotos")
	shared.LoadEnv()
	var db = shared.ConnectToDb()

	var err error
	sendPhotosBot, err = tgbotapi.NewBotAPI(os.Getenv("SEND_PHOTOS_BOT_TOKEN"))
	parseSiteBot, err = tgbotapi.NewBotAPI(os.Getenv("PARSE_SITE_BOT_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}
	parseChannelId, _ = strconv.ParseInt(os.Getenv("PARSE_CHANNEL_ID"), 10, 64)

	parseSiteBot.SetWebhook(tgbotapi.NewWebhook("https://richinme.com/go/elitebabes/parse_photos/" + parseSiteBot.Token))

	updates := parseSiteBot.ListenForWebhook("/go/elitebabes/parse_photos/" + parseSiteBot.Token)
	go http.ListenAndServe(":8001", nil)

	for update := range updates {
		if update.Message != nil {
			if isValidUrl(update.Message.Text) {
				if linkUrlExists(db, update.Message.Text) {
					continue
				}
				parseUrl(db, update)
			} else {
				var state = elite_model.State{}
				var err = db.Get(&state, "SELECT link_id, state_type FROM states WHERE user_id=$1 LIMIT 1", update.Message.From.ID)
				if err != nil {
					reSend(parseSiteBot, tgbotapi.NewEditMessageText(parseChannelId, update.CallbackQuery.Message.MessageID,
						"Ты не найден в базе"))
					continue
				} else {
					var link = elite_model.Link{}
					db.Get(&link, "SELECT id, status FROM links WHERE id=$1 LIMIT 1", state.LinkId)
					if link.Status > 0 {
						sendSimpleMessage("поздно что-либо менять")
						continue
					} else {
						var fieldName string
						if state.StateType == ActionNameId {
							fieldName = "model"
						} else if state.StateType == ActionDescriptionId {
							fieldName = "description"
						}
						db.QueryRowx(`UPDATE links SET `+fieldName+` = $1 where id = $2`, update.Message.Text, link.Id)
						sendSimpleMessage(fmt.Sprintf("Поле %s изменено", fieldName))
						continue
					}
				}

			}
		} else if update.CallbackQuery != nil {
			var callback Callback
			json.Unmarshal([]byte(update.CallbackQuery.Data), &callback)

			if callback.LinkId > 0 && callback.ActionId > 0 {
				if !linkIdExists(db, callback.LinkId) {
					sendSimpleMessage("Подборка удалена")
					continue
				}
				changeState(db, update.CallbackQuery.From.ID, callback.LinkId, callback.ActionId)
			}

			if callback.ActionId == ActionToggleId {
				toggleButtons(db, callback.MediaIds)
			} else if callback.ActionId == ActionPublicId {
				public(db, callback.LinkId)
				removeAction(update, "Опубликовано")
			} else if callback.ActionId == ActionDeleteId {
				removeAction(update, "Удалено")
			} else if callback.ActionId == ActionNameId {
				sendSimpleMessage("Введи имя")
			} else if callback.ActionId == ActionDescriptionId {
				sendSimpleMessage("Введите описание")
			}
		}
	}
}

func linkIdExists(db *sqlx.DB, linkId int) bool {
	if db.Get(&elite_model.Link{}, "SELECT id FROM links WHERE id=$1 LIMIT 1", linkId) != nil {
		return false
	}
	return true
}

func removeAction(update tgbotapi.Update, text string) {
	config := tgbotapi.NewEditMessageText(parseChannelId,
		update.CallbackQuery.Message.MessageID,
		text)
	reSend(parseSiteBot, config)
}

func public(db *sqlx.DB, linkId int) {
	db.QueryRowx(`UPDATE links SET status = $1 where id = $2`, ActionPublicId, linkId)
}

func toggleButtons(db *sqlx.DB, mediaIds []int) {
	var mediasForActivate []elite_model.Media
	db.Select(&mediasForActivate, "SELECT id, message_id, link_id, message_id, row FROM media WHERE id=any($1) "+
		"and row = 0 order by id", pq.Array(mediaIds))
	if mediasForActivate != nil {
		for _, media := range mediasForActivate {
			db.QueryRowx(`UPDATE media SET row = (select max(row) + 1 as max_row
				FROM media WHERE link_id = $1) where id = $2`, media.LinkId, media.Id)
		}
		updateButtons(db, []elite_model.Media{mediasForActivate[0]})
	} else {
		var mediasForDeactivate []elite_model.Media
		var err = db.Select(&mediasForDeactivate, "SELECT id, message_id, link_id, row, message_id FROM media "+
			"WHERE id=any($1) and row > 0 order by row desc", pq.Array(mediaIds))
		if err != nil || mediasForDeactivate == nil {
			return
		}
		var minRow = 999
		for _, media := range mediasForDeactivate {
			if media.Row < minRow {
				minRow = media.Row
			}
		}

		var messagesForUpdate []elite_model.Media
		db.Select(&messagesForUpdate, "SELECT message_id from media WHERE link_id = $1 and row >= $2"+
			"group by message_id", mediasForDeactivate[0].LinkId, minRow)

		//var messageIdsForUpdate []int
		for _, media := range mediasForDeactivate {
			var _, _ = db.Queryx(`UPDATE media SET row = row-1 WHERE link_id = $1 and row > $2`,
				media.LinkId, media.Row)
			db.QueryRowx(`UPDATE media SET row = 0 where id = $1`, media.Id)
		}
		updateButtons(db, messagesForUpdate)
	}
}

func updateButtons(db *sqlx.DB, mediasForUpdate []elite_model.Media) {
	for _, mediaForUpdate := range mediasForUpdate {
		var medias []elite_model.Media
		db.Select(&medias, `SELECT id, row from media WHERE message_id = $1 order by id`, mediaForUpdate.MessageId)

		var buttons []tgbotapi.InlineKeyboardButton
		var buttonText string
		var mediaIds []int
		for _, media := range medias {
			if media.Row == 0 {
				buttonText = "➖"
			} else {
				buttonText = fmt.Sprintf("✅ %d", media.Row)
			}
			data := fmt.Sprintf(`{"media_id": [%d], "action_id": %d}`, media.Id, ActionToggleId)
			buttons = append(buttons, tgbotapi.InlineKeyboardButton{
				Text:         buttonText,
				CallbackData: &data,
			})
			mediaIds = append(mediaIds, media.Id)
		}

		keyboardConfig := tgbotapi.NewEditMessageReplyMarkup(parseChannelId,
			mediaForUpdate.MessageId,
			tgbotapi.InlineKeyboardMarkup{
				InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
					{
						tgbotapi.NewInlineKeyboardButtonData("Все",
							fmt.Sprintf(`{"media_id": %s, "action_id": %d}`,
								strings.Join(strings.Fields(fmt.Sprint(mediaIds)), ","), ActionToggleId)),
					},
					buttons,
				},
			})
		reSend(parseSiteBot, keyboardConfig)
	}
}

type Callback struct {
	MediaIds []int `json:"media_id"`
	LinkId   int   `json:"link_id"`
	ActionId int   `json:"action_id"`
}

func linkUrlExists(db *sqlx.DB, url string) bool {
	link := elite_model.Link{}
	err := db.Get(&link, "SELECT id, link, status, model, description FROM links WHERE link=$1 LIMIT 1", url)
	if err == nil {
		if link.Status > 0 {
			sendSimpleMessage("Уже есть в базе")
			return true
		} else {
			db.QueryRowx(`DELETE FROM links where id = $1`,
				link.Id)
		}
	}
	return false
}

func sendSimpleMessage(text string) tgbotapi.Message {
	config := tgbotapi.NewMessage(parseChannelId, text)
	config.BaseChat.DisableNotification = true
	return reSend(parseSiteBot, config)
}

func isValidUrl(path string) bool {
	_, err := url.ParseRequestURI(path)
	if err != nil {
		return false
	}

	u, err := url.Parse(path)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	return true
}

func parseUrl(db *sqlx.DB, update tgbotapi.Update) {
	var link = update.Message.Text

	doc, err := htmlquery.LoadURL(link)
	if err != nil {
		return
	}

	names := htmlquery.Find(doc, "//div[@class='link-btn']//h2//a/text()")
	var linkId int

	var description = translate(htmlquery.InnerText(htmlquery.FindOne(doc, "//h1[@class='header-inline']/text()")))

	db.QueryRowx(`INSERT INTO links (link, model, description) VALUES ($1, $2, $3) RETURNING id`,
		link, names[1].Data, description).Scan(&linkId)

	changeState(db, update.Message.From.ID, linkId, ActionToggleId)

	replacer := strings.NewReplacer(" ", "",
		"-", "",
		"+", "")

	chunkedPhotos := chunkBy(htmlquery.Find(doc, "//ul[@class='list-justified2']//li[a]//"+
		"img[contains(@srcset, '600w') or contains(@srcset, '800w')]//@srcset"), 5)
	for i1, photos := range chunkedPhotos {
		var files []interface{}
		for i2, photo := range photos {
			var photoUrl = strings.Split(strings.Split(photo.FirstChild.Data, ", ")[0], " ")[0]
			inpMedia := tgbotapi.NewInputMediaPhoto(photoUrl)
			if i1 == 0 && i2 == 0 {
				inpMedia.ParseMode = tgbotapi.ModeMarkdown
				inpMedia.Caption = fmt.Sprintf("*Модель:* #%s\n*Описание:* %s", replacer.Replace(names[1].Data), description)
			}
			files = append(files, inpMedia)
		}
		config := tgbotapi.NewMediaGroup(parseChannelId, files)
		config.BaseChat.DisableNotification = true
		var messages = reSendGroup(sendPhotosBot, config)

		var mediaIds []int
		for _, message := range messages {
			var mediaId int
			db.QueryRowx(`INSERT INTO media (link_id, file_id) VALUES ($1, $2) RETURNING id`,
				linkId, getFileIDFromMsg(message)).Scan(&mediaId)
			mediaIds = append(mediaIds, mediaId)
		}

		var keyboardConfig = tgbotapi.NewMessage(parseChannelId, "Выбери самые лучшие фото ⚙")
		var buttons []tgbotapi.InlineKeyboardButton
		for _, mediaId := range mediaIds {
			data := fmt.Sprintf(`{"media_id": [%d], "action_id": %d}`, mediaId, ActionToggleId)
			buttons = append(buttons, tgbotapi.InlineKeyboardButton{
				Text:         "➖",
				CallbackData: &data,
			})
		}
		keyboardConfig.ReplyMarkup = tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
				{
					tgbotapi.NewInlineKeyboardButtonData("Все",
						fmt.Sprintf(`{"media_id": %s, "action_id": %d}`,
							strings.Join(strings.Fields(fmt.Sprint(mediaIds)), ","), ActionToggleId)),
				},
				buttons,
			},
		}
		keyboardConfig.BaseChat.DisableNotification = true

		var message = reSend(parseSiteBot, keyboardConfig)
		db.QueryRowx(`UPDATE media SET message_id = $1 WHERE id = any($2)`, message.MessageID, pq.Array(mediaIds))
		time.Sleep(time.Second * time.Duration(1))
	}

	keyboardConfigPublish := tgbotapi.NewMessage(parseChannelId, "Действие")
	keyboardConfigPublish.BaseChat.DisableNotification = true
	removeLink := fmt.Sprintf(`{"link_id": %d, "action_id": %d}`, linkId, ActionDeleteId)
	publicLink := fmt.Sprintf(`{"link_id": %d, "action_id": %d}`, linkId, ActionPublicId)
	modelNameLink := fmt.Sprintf(`{"link_id": %d, "action_id": %d}`, linkId, ActionNameId)
	descriptionLink := fmt.Sprintf(`{"link_id": %d, "action_id": %d}`, linkId, ActionDescriptionId)
	keyboardConfigPublish.ReplyMarkup = tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
			{
				{Text: "📒 Задать имя", CallbackData: &modelNameLink},
				{Text: "📖 Задать Описание", CallbackData: &descriptionLink},
			},
			{
				{Text: "❌ Удалить", CallbackData: &removeLink},
				{Text: "✅ Опубликовать", CallbackData: &publicLink},
			},
		},
	}
	reSend(parseSiteBot, keyboardConfigPublish)
}

func changeState(db *sqlx.DB, userId int, linkId int, stateType int) {
	err := db.QueryRowx(`INSERT INTO states (user_id, link_id, state_type) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET 
			link_id = EXCLUDED.link_id,
			state_type = EXCLUDED.state_type`, userId, linkId, stateType)
	if err != nil {
		fmt.Println(err)
	}
}

func reSend(bot *tgbotapi.BotAPI, c tgbotapi.Chattable) tgbotapi.Message {
	var resp, err = bot.Send(c)
	if err != nil {
		var botError = err.(*tgbotapi.Error)
		if botError.RetryAfter > 0 {
			time.Sleep(time.Second * (time.Duration(botError.RetryAfter) + 1))
			return reSend(bot, c)
		}
	}
	return resp
}

func reSendGroup(bot *tgbotapi.BotAPI, c tgbotapi.Chattable) []tgbotapi.Message {
	var resp, err = bot.SendGroup(c)
	if err != nil {
		var botError = err.(*tgbotapi.Error)
		if botError.RetryAfter > 0 {
			time.Sleep(time.Second * (time.Duration(botError.RetryAfter) + 1))
			return reSendGroup(bot, c)
		}
	}
	return resp
}

func getFileIDFromMsg(message tgbotapi.Message) string {
	return (*message.Photo)[len(*message.Photo)-1].FileID
}

func chunkBy(items []*html.Node, chunkSize int) (chunks [][]*html.Node) {
	for chunkSize < len(items) {
		items, chunks = items[chunkSize:], append(chunks, items[0:chunkSize:chunkSize])
	}

	return append(chunks, items)
}

func translate(text string) string {
	if text == "" {
		return ""
	}
	api := "https://translate.google.com/translate_a/single?client=at&dt=t&dt=ld&dt=qca&dt=rm&dt=bd&dj=1&hl=uk-RU" +
		"&ie=UTF-8&oe=UTF-8&inputm=2&otf=2&iid=1dd3b944-fa62-4b55-b330-74909a99969e&" +
		"sl=en&tl=ru&q=" + url.QueryEscape(text)
	fmt.Println(api)
	client := &http.Client{}
	req, _ := http.NewRequest("GET", api, nil)
	req.Header.Set("User-Agent", "AndroidTranslate/5.3.0.RC02.130475354-53000263 5.1 phone TRANSLATE_OPM5_TEST_1")
	req.Header.Set("Host", "translate.google.com")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	resultJson := make(map[string]interface{})
	err = json.Unmarshal(buf, &resultJson)
	if err != nil {
		return ""
	}

	sentences, _ := resultJson["sentences"].([]interface{})
	var output []string
	for _, sentence := range sentences {
		line := sentence.(map[string]interface{})["trans"]
		if line == nil {
			continue
		}
		output = append(output, line.(string))
	}
	return strings.Join(output, "")
}
