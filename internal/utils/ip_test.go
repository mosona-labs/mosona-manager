package utils

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/oschwald/geoip2-golang"
)

func TestMain(m *testing.M) {
	db, err := geoip2.Open(filepath.Join("..", "..", "GeoLite2-Country.mmdb"))
	if err != nil {
		panic(err)
	}
	geoDbMu.Lock()
	geoDb = db
	geoDbMu.Unlock()

	code := m.Run()

	geoDbMu.Lock()
	geoDb = nil
	geoDbMu.Unlock()
	_ = db.Close()
	os.Exit(code)
}

func TestGetIPGeoLocationCloudflare(t *testing.T) {
	geo, err := GetIPGeoLocation("1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if geo.Country != "Australia" || geo.CountryCode != "AU" {
		t.Fatalf("got %#v", geo)
	}
}

func TestGetIPGeoLocationGoogle(t *testing.T) {
	geo, err := GetIPGeoLocation("8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if geo.Country != "United States" || geo.CountryCode != "US" {
		t.Fatalf("got %#v", geo)
	}
}

func TestGetIPGeoLocationPrivate(t *testing.T) {
	geo, err := GetIPGeoLocation(net.IPv4(192, 168, 1, 1).String())
	if err != nil {
		t.Fatal(err)
	}
	if geo.Country != "Private Network" || geo.CountryCode != "UN" {
		t.Fatalf("got %#v", geo)
	}
}
