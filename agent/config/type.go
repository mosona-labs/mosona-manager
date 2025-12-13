package config

type Config struct {
	Mode string `json:"mode"` // active | passive

	// Passive
	Hub  string `json:"hub,omitempty"`
	UUID string `json:"uuid,omitempty"`
}
