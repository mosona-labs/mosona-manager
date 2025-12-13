package config

type Config struct {
	Mode string `json:"mode"` // active | passive

	NoMonitor  bool `json:"no_monitor"`
	NoTerminal bool `json:"no_terminal"`

	// Passive
	Hub  string `json:"hub,omitempty"`
	UUID string `json:"uuid,omitempty"`
}
