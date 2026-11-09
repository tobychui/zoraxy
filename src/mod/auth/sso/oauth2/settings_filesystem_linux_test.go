//go:build linux && (mipsle || riscv64 || fsdb)

package oauth2

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/oauth2"
)

func TestFilesystemFailedWriteRetainsPolicyAcrossRestart(t *testing.T) {
	router := newFilesystemSettingsRouter(t)
	userinfo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"sub":"alice","groups":["guests"]}`))
	}))
	defer userinfo.Close()
	saveTestSettings(t, router, userinfo.URL, "admins")
	path := filepath.Join(router.options.Database.Db.(string), "oauth2", "settings.entry")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// A real Linux short/failed file write, without replacing Database.Backend.
	// The process limit is restored before assertions or any subsequent tests.
	w := httptest.NewRecorder()
	func() {
		var original syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &original); err != nil {
			t.Fatal(err)
		}
		signal.Ignore(syscall.SIGXFSZ)
		defer signal.Reset(syscall.SIGXFSZ)
		limited := original
		limited.Cur = 16
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limited); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &original); err != nil {
				t.Fatal(err)
			}
		}()
		router.HandleSetOAuth2Settings(w, settingsRequest(""))
	}()
	if !strings.Contains(w.Body.String(), "error") {
		t.Fatalf("failed write reported success: %s", w.Body.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed write damaged saved policy: %s, %v", after, err)
	}
	for _, current := range []*OAuth2Router{router, restartSettingsTestRouter(t, router)} {
		r := httptest.NewRequest(http.MethodGet, "https://app.test/private", nil)
		_, options := current.requestConfiguration(r.Host, "https://app.test/internal/oauth2")
		cookie, err := current.createSession(r, &oauth2.Token{AccessToken: "valid"}, "alice", options.settingsGeneration)
		if err != nil {
			t.Fatal(err)
		}
		r.AddCookie(cookie)
		response := httptest.NewRecorder()
		_ = current.HandleOAuth2Auth(response, r, nil)
		if response.Code != http.StatusForbidden {
			t.Fatalf("previous group restriction not enforced: %d", response.Code)
		}
	}
}
