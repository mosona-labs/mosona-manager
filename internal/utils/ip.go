package utils

import (
	"encoding/json"
	"errors"
	"mosona-manager/internal/runtime"
	"net"
	"net/http"
	"sync"
	"time"
)

var privateCIDRs []*net.IPNet

func init() {
	cidr := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}
	for _, c := range cidr {
		_, n, err := net.ParseCIDR(c)
		if err == nil {
			privateCIDRs = append(privateCIDRs, n)
		}
	}
}

type IPGeoResponse struct {
	Country         string  `json:"country"`
	CountryCode     string  `json:"country_code"`
	ASN             int     `json:"asn"`
	ASNOrganization string  `json:"asn_organization"`
	ISP             string  `json:"isp"`
	Organization    string  `json:"organization"`
	Latitude        float64 `json:"latitude"`
	Longitude       float64 `json:"longitude"`
	Timezone        string  `json:"timezone"`
	IP              string  `json:"ip"`
	ContinentCode   string  `json:"continent_code"`
}

func GetDomainAddress(domain string) (string, error) {
	ips, err := net.LookupIP(domain)
	if err != nil {
		return "", err
	}
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			return ipv4.String(), nil
		}
		if ipv6 := ip.To16(); ipv6 != nil {
			return ipv6.String(), nil
		}
	}
	return "", errors.New("domain not found")
}

func IsPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range privateCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

type cacheEntry struct {
	Data   IPGeoResponse
	Expiry time.Time
}

var (
	ipCache         = make(map[string]cacheEntry)
	ipCacheMu       sync.RWMutex
	cacheTTL        = 24 * time.Hour
	janitorInterval = time.Hour
	cacheOnce       sync.Once
)

func startCacheJanitor() {
	go func() {
		ticker := time.NewTicker(janitorInterval)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			ipCacheMu.Lock()
			for k, v := range ipCache {
				if now.After(v.Expiry) {
					delete(ipCache, k)
				}
			}
			ipCacheMu.Unlock()
		}
	}()
}

func GetIPGeoLocation(ip string) (IPGeoResponse, error) {
	cacheOnce.Do(startCacheJanitor)

	ipCacheMu.RLock()
	if e, ok := ipCache[ip]; ok {
		if time.Now().Before(e.Expiry) {
			ipCacheMu.RUnlock()
			return e.Data, nil
		}
	}
	ipCacheMu.RUnlock()

	if IsPrivateIP(ip) {
		res := IPGeoResponse{
			Country:     "Private Network",
			CountryCode: "UN",
			IP:          ip,
		}
		ipCacheMu.Lock()
		ipCache[ip] = cacheEntry{Data: res, Expiry: time.Now().Add(cacheTTL)}
		ipCacheMu.Unlock()
		return res, nil
	}

	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://api.ip.sb/geoip/"+ip, nil)
	if err != nil {
		return IPGeoResponse{}, err
	}
	req.Header.Set("User-Agent", "mosona-manager-hub/"+runtime.Version)

	resp, err := client.Do(req)
	if err != nil {
		return IPGeoResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return IPGeoResponse{}, errors.New("failed to get geo location")
	}

	var geoData IPGeoResponse
	if err = json.NewDecoder(resp.Body).Decode(&geoData); err != nil {
		return IPGeoResponse{}, err
	}

	ipCacheMu.Lock()
	ipCache[ip] = cacheEntry{Data: geoData, Expiry: time.Now().Add(cacheTTL)}
	ipCacheMu.Unlock()

	return geoData, nil
}
