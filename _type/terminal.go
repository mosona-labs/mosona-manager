package _type

type Terminal struct {
	ServerMinimal
	Username string  `db:"username" json:"username"`
	Address  string  `db:"address" json:"address"`
	Port     int     `db:"port" json:"port"`
	OS       *string `json:"os"`
}

type TerminalDetail struct {
	Address  string `db:"address"`
	Port     int    `db:"port"`
	Username string `db:"username"`
	Password string `db:"password,omitempty"`
}
