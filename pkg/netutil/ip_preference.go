package netutil

import "fmt"

const (
	IPPreferenceAuto = ""
	IPPreferenceIPv4 = "ipv4"
	IPPreferenceIPv6 = "ipv6"
)

func NetworkForIPPreference(preference string) (string, error) {
	switch preference {
	case IPPreferenceAuto:
		return "tcp", nil
	case IPPreferenceIPv4:
		return "tcp4", nil
	case IPPreferenceIPv6:
		return "tcp6", nil
	default:
		return "", fmt.Errorf("invalid IP preference: %s", preference)
	}
}
