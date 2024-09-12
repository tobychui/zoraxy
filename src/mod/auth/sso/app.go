package sso

/*
	app.go

	This file contains the app structure and app management
	functions for the SSO module.

*/

// RegisteredUpstreamApp is a structure that contains the information of an
// upstream app that is registered with the SSO server
type RegisteredUpstreamApp struct {
	ID              string
	Secret          string
	Domain          []string
	Scopes          []string
	SessionDuration int //in seconds, default to 1 hour
}

// RegisterUpstreamApp registers an upstream app with the SSO server
func (s *SSOHandler) ListRegisteredApps() []*RegisteredUpstreamApp {
	apps := make([]*RegisteredUpstreamApp, 0)
	for _, app := range s.Apps {
		apps = append(apps, &app)
	}
	return apps
}

// RegisterUpstreamApp registers an upstream app with the SSO server
func (s *SSOHandler) GetAppByID(appID string) (*RegisteredUpstreamApp, bool) {
	app, ok := s.Apps[appID]
	return &app, ok
}
