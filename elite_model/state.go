package elite_model

type State struct {
	UserId    int `db:"user_id"`
	LinkId    int `db:"link_id"`
	StateType int `db:"state_type"`
}
