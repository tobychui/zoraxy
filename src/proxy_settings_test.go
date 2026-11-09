package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
)

// Pause a handler after it has saved, but before its administrative transaction
// ends. A competing editor must not read an endpoint until that transaction ends.
type pausedSettingsResponse struct {
	*httptest.ResponseRecorder
	entered chan struct{}
	release chan struct{}
}

func (w *pausedSettingsResponse) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return w.ResponseRecorder.Write(p)
}

func TestProxyEditorsSerializeWithOAuth2Settings(t *testing.T) {
	for _, editor := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"tags", ReverseProxyHandleSetTags},
		{"main", ReverseProxyHandleEditEndpoint},
	} {
		for _, oauthFirst := range []bool{false, true} {
			order := "editor-first"
			if oauthFirst {
				order = "oauth-first"
			}
			t.Run(editor.name+"/"+order, func(t *testing.T) {
				oldRouter, oldBalancer, oldFolder := dynamicProxyRouter, loadBalancer, CONF_HTTP_PROXY
				t.Cleanup(func() {
					dynamicProxyRouter, loadBalancer, CONF_HTTP_PROXY = oldRouter, oldBalancer, oldFolder
				})
				dynamicProxyRouter = &dynamicproxy.Router{ProxyEndpoints: &sync.Map{}}
				loadBalancer = &loadbalance.RouteManager{}
				CONF_HTTP_PROXY = t.TempDir()
				endpoint := &dynamicproxy.ProxyEndpoint{
					RootOrMatchingDomain: "app.test",
					ProxyType:            dynamicproxy.ProxyTypeHost,
					AuthenticationProvider: &dynamicproxy.AuthenticationProvider{
						AuthMethod: dynamicproxy.AuthMethodOauth2,
						OAuth2ExceptionRules: []*dynamicproxy.BasicAuthExceptionRule{{
							RuleType: dynamicproxy.AuthExceptionType_Paths, PathPrefix: "/public",
						}},
					},
				}
				if _, err := dynamicProxyRouter.PrepareProxyRoute(endpoint); err != nil {
					t.Fatal(err)
				}
				if err := dynamicProxyRouter.AddProxyRouteToRuntime(endpoint); err != nil {
					t.Fatal(err)
				}
				request := func(form url.Values) *http.Request {
					r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
					r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					return r
				}
				oauthRequest := request(url.Values{
					"ep":       {"app.test"},
					"settings": {`{"allowedGroups":"admins","exceptionRules":[],"forwardUserInfo":true}`},
				})
				editRequest := request(url.Values{
					"rootname": {"app.test"}, "tags": {"updated"}, "authprovider": {"3"},
				})
				first, second := editor.handler, http.HandlerFunc(HandleProxyOAuth2Settings)
				firstRequest, secondRequest := editRequest, oauthRequest
				if oauthFirst {
					first, second = second, first
					firstRequest, secondRequest = secondRequest, firstRequest
				}
				paused := &pausedSettingsResponse{httptest.NewRecorder(), make(chan struct{}), make(chan struct{})}
				firstDone := make(chan struct{})
				go func() {
					defer close(firstDone)
					first(paused, firstRequest)
				}()
				<-paused.entered
				secondResponse := httptest.NewRecorder()
				secondStarted, secondDone := make(chan struct{}), make(chan struct{})
				go func() {
					defer close(secondDone)
					close(secondStarted)
					second(secondResponse, secondRequest)
				}()
				<-secondStarted
				select {
				case <-secondDone:
					t.Error("competing editor completed while an endpoint transaction was still active")
				case <-time.After(50 * time.Millisecond):
				}
				close(paused.release)
				<-firstDone
				<-secondDone
				for _, response := range []*httptest.ResponseRecorder{paused.ResponseRecorder, secondResponse} {
					if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "error") {
						t.Fatalf("save failed: %d %s", response.Code, response.Body.String())
					}
				}
				current, err := dynamicProxyRouter.LoadProxy("app.test")
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(CONF_HTTP_PROXY, "app.test.config"))
				if err != nil {
					t.Fatal(err)
				}
				var persisted dynamicproxy.ProxyEndpoint
				if err := json.Unmarshal(data, &persisted); err != nil {
					t.Fatal(err)
				}
				for _, got := range []*dynamicproxy.ProxyEndpoint{current, &persisted} {
					policy := got.AuthenticationProvider
					if policy.AuthMethod != dynamicproxy.AuthMethodOauth2 || len(policy.OAuth2ExceptionRules) != 0 || !reflect.DeepEqual(policy.OAuth2AllowedGroups, []string{"admins"}) {
						t.Errorf("OAuth2 restriction was lost: %+v", policy)
					}
					if !reflect.DeepEqual(got.Tags, []string{"updated"}) {
						t.Errorf("editor update was lost: %v", got.Tags)
					}
				}
			})
		}
	}
}
