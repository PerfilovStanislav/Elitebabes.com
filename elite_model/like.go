package elite_model

type Like struct {
	Id        int
	ChatID    int `db:"chat_id"`
	FromId    int `db:"from_id"`
	MessageId int `db:"message_id"`
	Like      bool
}
