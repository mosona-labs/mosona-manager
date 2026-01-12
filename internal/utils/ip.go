package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
)

var (
	geoDb   *geoip2.Reader
	geoDbMu sync.RWMutex

	privateCIDRs []*net.IPNet
)

func init() {
	// load existing DB if present
	if _, err := os.Stat(`GeoLite2-Country.mmdb`); err == nil {
		if db, err := geoip2.Open(`GeoLite2-Country.mmdb`); err == nil {
			geoDbMu.Lock()
			geoDb = db
			geoDbMu.Unlock()
		} else {
			fmt.Println("open geo db:", err)
		}
	}

	// periodic updater
	go func() {
		for {
			downloadGeoLite2()
			time.Sleep(7 * 24 * time.Hour) // weekly
		}
	}()

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

func downloadGeoLite2() {
	tmp := `GeoLite2-Country.mmdb.tmp`
	dst := `GeoLite2-Country.mmdb`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://git.io/GeoLite2-Country.mmdb", nil)
	if err != nil {
		return
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// write to temp file
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	_, err = io.Copy(f, resp.Body)
	_ = f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return
	}

	// replace DB file
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return
	}

	// open new DB and swap atomically
	if db, err := geoip2.Open(dst); err == nil && db != nil {
		geoDbMu.Lock()
		if geoDb != nil {
			_ = geoDb.Close()
		}
		geoDb = db
		geoDbMu.Unlock()
	} else {
		// cleanup on failure
		if db != nil {
			_ = db.Close()
		}
	}
}

type IPGeoResponse struct {
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	IP          string `json:"ip"`
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

func GetIPGeoLocation(ip string) (IPGeoResponse, error) {
	if IsPrivateIP(ip) {
		res := IPGeoResponse{
			Country:     "Private Network",
			CountryCode: "UN",
			IP:          ip,
		}
		return res, nil
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return IPGeoResponse{}, errors.New("invalid ip")
	}

	geoDbMu.RLock()
	db := geoDb
	geoDbMu.RUnlock()

	if db == nil {
		return IPGeoResponse{}, errors.New("geo database not available")
	}

	record, err := db.Country(parsed)
	if err != nil {
		return IPGeoResponse{}, err
	}

	return IPGeoResponse{
		Country:     record.Country.Names["en"],
		CountryCode: record.Country.IsoCode,
		IP:          ip,
	}, nil
}
