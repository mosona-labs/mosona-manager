package _type

type UsersConfig struct {
	UID        int64 `json:"uid"`
	ActiveTeam int64 `json:"active_team" db:"active_team"`
}
