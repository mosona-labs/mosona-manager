package _type

type SessionData struct {
	ID        string `json:"id"`
	UID       int64  `json:"uid"`
	TID       int64  `json:"tid"`
	UserAgent string `json:"user_agent"`
	Time      int64  `json:"time"`
}
