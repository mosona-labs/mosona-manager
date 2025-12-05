package oauth

import (
	"fmt"
	"log"
	"mosona-manager/_type"
	"mosona-manager/config"
	"mosona-manager/db"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type ProviderConfig struct {
	Config      *oauth2.Config
	UserinfoUrl string
	Skip        bool
}

var (
	Configs      = make(map[int]*ProviderConfig)
	providerLock sync.RWMutex
)

func Init() {
	oauthList, err := db.GetOAuthProvider()
	if err != nil {
		log.Fatalln("Init auth provider error:", err)
	}

	var tempConfigs = make(map[int]*ProviderConfig)
	for _, oauth := range oauthList {
		oauthConfig := &oauth2.Config{
			ClientID:     oauth.ClientID,
			ClientSecret: oauth.ClientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  oauth.AuthUrl,
				TokenURL: oauth.TokenUrl,
			},
			RedirectURL: fmt.Sprintf("%s/oauth/%d", config.DynamicConf.Domain, oauth.ID),
			Scopes:      strings.Fields("read:user read:email"),
		}
		tempConfigs[oauth.ID] = &ProviderConfig{
			Config:      oauthConfig,
			UserinfoUrl: oauth.UserinfoUrl,
			Skip:        oauth.Skip2FA,
		}
	}

	providerLock.Lock()
	Configs = tempConfigs
	providerLock.Unlock()
}

func AddProvider(oauth _type.AuthProvider) error {
	oauthConfig := &oauth2.Config{
		ClientID:     oauth.ClientID,
		ClientSecret: oauth.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  oauth.AuthUrl,
			TokenURL: oauth.TokenUrl,
		},
		RedirectURL: fmt.Sprintf("%s/oauth/%d", config.DynamicConf.Domain, oauth.ID),
		Scopes:      []string{oidc.ScopeOpenID, "profile", "email"},
	}

	providerLock.Lock()
	Configs[oauth.ID] = &ProviderConfig{
		Config:      oauthConfig,
		UserinfoUrl: oauth.UserinfoUrl,
		Skip:        oauth.Skip2FA,
	}
	providerLock.Unlock()

	return nil
}

func RemoveProvider(providerID int) {
	providerLock.Lock()
	delete(Configs, providerID)
	providerLock.Unlock()
}

func GetProviders(providerID int) (*ProviderConfig, bool) {
	providerLock.RLock()
	defer providerLock.RUnlock()

	conf, ok := Configs[providerID]
	if !ok {
		return nil, false
	}

	return conf, len(Configs) > 0
}
