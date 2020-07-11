package elite_model

type Media struct {
	Id        int
	LinkId    int    `db:"link_id"`
	FileId    string `db:"file_id"`
	Row       int
	MessageId int `db:"message_id"`
}
