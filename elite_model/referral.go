package elite_model

type Referral struct {
	ParentId int `db:"parent_id"`
	UserId   int `db:"user_id"`
}
