package utils

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
)

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

func GetIPGeoLocation(ip string) (IPGeoResponse, error) {
	client := &http.Client{}

	req, err := http.NewRequest("GET", "https://api.ip.sb/geoip/"+ip, nil)
	if err != nil {
		return IPGeoResponse{}, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; YourAppName/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return IPGeoResponse{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return IPGeoResponse{}, errors.New("failed to get geo location")
	}

	var geoData IPGeoResponse
	if err = json.NewDecoder(resp.Body).Decode(&geoData); err != nil {
		return IPGeoResponse{}, err
	}

	return geoData, nil
}
