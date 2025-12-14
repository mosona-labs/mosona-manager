package _type

type Terminal struct {
	ServerMinimal
	OS *string `json:"os"`

	// SSH
	Username *string `db:"username" json:"username,omitempty"`
	Address  *string `db:"address" json:"address,omitempty"`
	Port     *int    `db:"port" json:"port,omitempty"`
}

type TerminalDetail struct {
	Type int16 `json:"type"`

	// SSH
	Address  *string `db:"address"`
	Port     *int    `db:"port"`
	Username *string `db:"username"`
	Password *string `db:"password"`
	Key      *string `db:"key"`
	KeyPwd   *string `db:"key_pwd"`
}
