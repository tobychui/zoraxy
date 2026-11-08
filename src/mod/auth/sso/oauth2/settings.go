package oauth2

import (
	"errors"
	"time"

	"imuslab.com/zoraxy/mod/database/dbinc"
)

// Keep policy and provider settings in one database value, so a failed write
// cannot commit only part of an administrative change.
const oauth2SettingsKey = "settings"

type persistedOAuth2Settings struct {
	OAuth2WellKnownUrl           string         `json:"oauth2WellKnownUrl"`
	OAuth2ServerURL              string         `json:"oauth2ServerUrl"`
	OAuth2TokenURL               string         `json:"oauth2TokenUrl"`
	OAuth2ClientId               string         `json:"oauth2ClientId"`
	OAuth2ClientSecret           string         `json:"oauth2ClientSecret"`
	OAuth2UserInfoUrl            string         `json:"oauth2UserInfoUrl"`
	OAuth2CodeChallengeMethod    string         `json:"oauth2CodeChallengeMethod"`
	OAuth2Scopes                 string         `json:"oauth2Scopes"`
	OAuth2DefaultAllowedGroups   string         `json:"oauth2DefaultAllowedGroups"`
	OAuth2ConfigurationCacheTime *time.Duration `json:"oauth2ConfigurationCacheTime"`
}

func (settings persistedOAuth2Settings) apply(options *OAuth2RouterOptions) {
	options.OAuth2WellKnownUrl = settings.OAuth2WellKnownUrl
	options.OAuth2ServerURL = settings.OAuth2ServerURL
	options.OAuth2TokenURL = settings.OAuth2TokenURL
	options.OAuth2ClientId = settings.OAuth2ClientId
	options.OAuth2ClientSecret = settings.OAuth2ClientSecret
	options.OAuth2UserInfoUrl = settings.OAuth2UserInfoUrl
	options.OAuth2CodeChallengeMethod = settings.OAuth2CodeChallengeMethod
	options.OAuth2Scopes = settings.OAuth2Scopes
	options.OAuth2DefaultAllowedGroups = settings.OAuth2DefaultAllowedGroups
	options.OAuth2ConfigurationCacheTime = settings.OAuth2ConfigurationCacheTime
}

func loadOAuth2Settings(options *OAuth2RouterOptions) {
	var settings persistedOAuth2Settings
	err := options.Database.Read("oauth2", oauth2SettingsKey, &settings)
	if err == nil {
		settings.apply(options)
		return
	}
	failClosed := func(err error) {
		options.Logger.PrintAndLog("OAuth2Router", "Failed to load OAuth2 settings", err)
		(persistedOAuth2Settings{}).apply(options)
	}
	if !errors.Is(err, dbinc.ErrNotFound) {
		failClosed(err)
		return
	}
	// Only confirmed absence permits legacy migration. Missing individual legacy
	// keys are allowed, but a corrupt or unreadable policy must not become empty.
	for _, entry := range []struct {
		key    string
		target interface{}
	}{
		{"oauth2WellKnownUrl", &options.OAuth2WellKnownUrl},
		{"oauth2ServerUrl", &options.OAuth2ServerURL},
		{"oauth2TokenUrl", &options.OAuth2TokenURL},
		{"oauth2ClientId", &options.OAuth2ClientId},
		{"oauth2ClientSecret", &options.OAuth2ClientSecret},
		{"oauth2UserInfoUrl", &options.OAuth2UserInfoUrl},
		{"oauth2CodeChallengeMethod", &options.OAuth2CodeChallengeMethod},
		{"oauth2Scopes", &options.OAuth2Scopes},
		{"oauth2DefaultAllowedGroups", &options.OAuth2DefaultAllowedGroups},
		{"oauth2ConfigurationCacheTime", &options.OAuth2ConfigurationCacheTime},
	} {
		if err := options.Database.Read("oauth2", entry.key, entry.target); err != nil && !errors.Is(err, dbinc.ErrNotFound) {
			failClosed(err)
			return
		}
	}
}

// Caller holds settingsMu through persistence, publication and invalidation.
func (ar *OAuth2Router) saveOAuth2Settings(settings persistedOAuth2Settings) error {
	if err := ar.options.Database.Write("oauth2", oauth2SettingsKey, settings); err != nil {
		return err
	}
	settings.apply(ar.options)
	ar.settingsGeneration++
	ar.sessions.DeleteAll()
	ar.options.OAuth2ConfigCache.DeleteAll()
	return nil
}
