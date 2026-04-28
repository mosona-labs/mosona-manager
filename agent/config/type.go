package config

type Config struct {
	Mode string `json:"mode"` // active | passive

	NoMonitor  bool `json:"no_monitor"`
	NoTerminal bool `json:"no_terminal"`

	IPPreference string `json:"ip_preference,omitempty"` // "" | ipv4 | ipv6

	// Active
	Uid  string `json:"uid,omitempty"`
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`

	// Passive
	Hub  string `json:"hub,omitempty"`
	UUID string `json:"uuid,omitempty"`
}
