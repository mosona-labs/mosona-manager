package githubrelease

import "fmt"

func AssetNameForPlatform(osName, arch string) (string, error) {
	if err := validatePlatform(osName, arch); err != nil {
		return "", err
	}
	name := fmt.Sprintf("agent_%s_%s", osName, arch)
	if osName == "windows" {
		name += ".exe"
	}
	return name, nil
}

func ChecksumNameForPlatform(osName, arch string) (string, error) {
	base, err := AssetNameForPlatform(osName, arch)
	if err != nil {
		return "", err
	}
	return base + ".sha256", nil
}

func validatePlatform(osName, arch string) error {
	switch osName {
	case "linux", "darwin", "windows":
	default:
		return fmt.Errorf("unsupported os: %s", osName)
	}
	switch arch {
	case "amd64", "arm64":
	default:
		return fmt.Errorf("unsupported arch: %s", arch)
	}
	return nil
}
