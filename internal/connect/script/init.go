package script

import "embed"

//go:embed *
var Scripts embed.FS

func GetScript(name string) (string, error) {
	data, err := Scripts.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
