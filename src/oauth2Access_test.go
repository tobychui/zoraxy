package main

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"imuslab.com/zoraxy/mod/dynamicproxy"
)

func TestUpdateProxyOAuth2SettingsPublishesImmutableReplacement(t *testing.T) {
	originalRule := &dynamicproxy.BasicAuthExceptionRule{RuleType: dynamicproxy.AuthExceptionType_Paths, PathPrefix: "/old"}
	original := &dynamicproxy.ProxyEndpoint{
		RootOrMatchingDomain: "example.test",
		AuthenticationProvider: &dynamicproxy.AuthenticationProvider{
			AuthMethod:           dynamicproxy.AuthMethodOauth2,
			OAuth2AllowedGroups:  []string{"old"},
			OAuth2ExceptionRules: []*dynamicproxy.BasicAuthExceptionRule{originalRule},
			ForwardAuthURL:       "https://forward-auth.example.test",
		},
	}
	inputRule := &dynamicproxy.BasicAuthExceptionRule{RuleType: dynamicproxy.AuthExceptionType_Paths, PathPrefix: "/public"}
	input := oauth2RouteSettings{
		AllowedGroups:                  "admins, developers",
		ExceptionRules:                 []*dynamicproxy.BasicAuthExceptionRule{inputRule},
		ForwardUserInfo:                true,
		RemoveUntrustedIdentityHeaders: true,
		ForwardWebSocketIdentity:       true,
	}

	var saved, published *dynamicproxy.ProxyEndpoint
	err := updateProxyOAuth2Settings(original, input, func(endpoint *dynamicproxy.ProxyEndpoint) error {
		saved = endpoint
		return nil
	}, func(endpoint *dynamicproxy.ProxyEndpoint) {
		published = endpoint
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved == nil || published == nil || saved != published {
		t.Fatal("complete replacement must be persisted before the same pointer is published")
	}
	if original.AuthenticationProvider.OAuth2AllowedGroups[0] != "old" || original.AuthenticationProvider.OAuth2ExceptionRules[0] != originalRule {
		t.Fatal("live endpoint was mutated while building replacement")
	}
	provider := published.AuthenticationProvider
	if strings.Join(provider.OAuth2AllowedGroups, ",") != "admins,developers" || provider.OAuth2ExceptionRules[0].PathPrefix != "/public" {
		t.Fatalf("unexpected replacement policy: %+v", provider)
	}
	if provider.ForwardAuthURL != original.AuthenticationProvider.ForwardAuthURL {
		t.Fatal("unrelated authentication settings were not preserved")
	}
	if provider.OAuth2ExceptionRules[0] == inputRule {
		t.Fatal("published exception rule aliases request-owned input")
	}
	inputRule.PathPrefix = "/mutated"
	if provider.OAuth2ExceptionRules[0].PathPrefix != "/public" {
		t.Fatal("published policy changed after input mutation")
	}
}

func TestUpdateProxyOAuth2SettingsPersistenceFailureDoesNotPublish(t *testing.T) {
	original := &dynamicproxy.ProxyEndpoint{
		RootOrMatchingDomain: "example.test",
		AuthenticationProvider: &dynamicproxy.AuthenticationProvider{
			AuthMethod:          dynamicproxy.AuthMethodOauth2,
			OAuth2AllowedGroups: []string{"old"},
		},
	}
	wantErr := errors.New("disk full")
	published := false
	err := updateProxyOAuth2Settings(original, oauth2RouteSettings{AllowedGroups: "new"}, func(*dynamicproxy.ProxyEndpoint) error {
		return wantErr
	}, func(*dynamicproxy.ProxyEndpoint) {
		published = true
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want %v", err, wantErr)
	}
	if published {
		t.Fatal("failed persistence must not publish a runtime-only policy")
	}
	if got := original.AuthenticationProvider.OAuth2AllowedGroups[0]; got != "old" {
		t.Fatalf("original policy changed to %q after failed save", got)
	}
}

func TestUpdateProxyOAuth2SettingsDoesNotRaceWithReaders(t *testing.T) {
	original := &dynamicproxy.ProxyEndpoint{
		RootOrMatchingDomain: "example.test",
		AuthenticationProvider: &dynamicproxy.AuthenticationProvider{
			AuthMethod:          dynamicproxy.AuthMethodOauth2,
			OAuth2AllowedGroups: []string{"readers"},
		},
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				_ = original.AuthenticationProvider.OAuth2AllowedGroups[0]
				_ = original.AuthenticationProvider.OAuth2DisableIdentityForwarding
			}
		}()
	}
	for i := 0; i < 1000; i++ {
		err := updateProxyOAuth2Settings(original, oauth2RouteSettings{AllowedGroups: "writers"}, func(*dynamicproxy.ProxyEndpoint) error {
			return nil
		}, func(*dynamicproxy.ProxyEndpoint) {})
		if err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}

func TestOAuth2AccessEditorInitializesAndRollsBack(t *testing.T) {
	content, err := os.ReadFile("web/snippet/oauth2AccessEditor.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(content)
	for _, expected := range []string{
		`#oauth2Advanced .ui.checkbox`,
		`let confirmed = null`,
		`rollbackFailedSave(data.error)`,
		`saved = cloneSettings(confirmed)`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("OAuth2 editor is missing regression guard %q", expected)
		}
	}
}
