package siteaccess

import (
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"strings"
	"sync"
)

type snapshot struct {
	baseHost    string
	publicHosts map[string]struct{}
}

var (
	mu   sync.RWMutex
	snap snapshot
)

func Refresh() error {
	baseHost := utils.SiteHostFromBaseURL(strings.TrimSpace(config.ReadDynamicConf().Domain))
	domains, err := db.ListEnabledPublicPageDomains()
	if err != nil {
		return err
	}
	hosts := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		h := utils.NormalizeRequestHost(d)
		if h == "" {
			continue
		}
		if baseHost != "" && strings.EqualFold(h, baseHost) {
			continue
		}
		hosts[h] = struct{}{}
	}
	mu.Lock()
	snap = snapshot{baseHost: baseHost, publicHosts: hosts}
	mu.Unlock()
	return nil
}

func BaseHost() string {
	mu.RLock()
	defer mu.RUnlock()
	return snap.baseHost
}

func IsPublicPageHost(host string) bool {
	h := utils.NormalizeRequestHost(host)
	if h == "" {
		return false
	}
	mu.RLock()
	base := snap.baseHost
	_, ok := snap.publicHosts[h]
	mu.RUnlock()
	if base != "" && strings.EqualFold(h, base) {
		return false
	}
	return ok
}

func HostAllowed(requestHost, configuredBaseURL string) bool {
	base := utils.SiteHostFromBaseURL(strings.TrimSpace(configuredBaseURL))
	if base == "" {
		return true
	}
	req := utils.NormalizeRequestHost(requestHost)
	if req == "" {
		return false
	}
	if strings.EqualFold(req, base) {
		return true
	}
	mu.RLock()
	_, ok := snap.publicHosts[req]
	mu.RUnlock()
	return ok
}

func PublicPagePathAllowed(path string) bool {
	if path == "/" || path == "/health" {
		return true
	}
	if strings.HasPrefix(path, "/api/public/") {
		return true
	}
	if strings.HasPrefix(path, "/preview-assets/") {
		return true
	}
	if strings.HasPrefix(path, "/avatars/") {
		return true
	}
	return false
}