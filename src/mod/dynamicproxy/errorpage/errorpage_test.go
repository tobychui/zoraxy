package errorpage

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	templateDir := filepath.Join(dir, "templates")
	m := NewManager(&Options{
		TemplateDir: templateDir,
		ConfigFile:  filepath.Join(dir, "error_pages.json"),
	})
	return m, templateDir
}

func TestValidateTemplateName(t *testing.T) {
	valid := []string{"404.html", "whitelist.html", "my-page_v2.html", "a.b.html"}
	for _, name := range valid {
		if err := ValidateTemplateName(name); err != nil {
			t.Errorf("expected %q to be valid, got %v", name, err)
		}
	}

	invalid := []string{
		"", ".html", "../404.html", "..\\404.html", "../../etc/passwd.html",
		"sub/404.html", "sub\\404.html", "/abs.html", "C:\\abs.html", "C:abs.html",
		".hidden.html", "a..html", "page.htm", "page", "page.html.exe",
		"con.html", "NUL.html", "com1.backup.html", "a b.html", "a%2f.html",
		"page.html\x00", "page:stream.html",
	}
	for _, name := range invalid {
		if err := ValidateTemplateName(name); err == nil {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func TestNormalizeTemplateName(t *testing.T) {
	cases := map[string]string{
		"mypage":       "mypage.html",
		" 404 ":        "404.html",
		"Page.HTML":    "Page.html",
		"page.html":    "page.html",
		"../x":         "../x.html",
		"weird.htm":    "weird.htm.html",
		"name.tmpl":    "name.tmpl.html",
		"already.Html": "already.html",
	}
	for input, expected := range cases {
		if got := NormalizeTemplateName(input); got != expected {
			t.Errorf("NormalizeTemplateName(%q) = %q, expected %q", input, got, expected)
		}
	}
}

func TestWriteRejectsPathEscape(t *testing.T) {
	m, templateDir := newTestManager(t)
	escapeNames := []string{"../escape.html", "..\\escape.html", "sub/escape.html", filepath.Join(templateDir, "..", "escape.html")}
	for _, name := range escapeNames {
		if err := m.WriteTemplate(name, []byte("x"), false); err == nil {
			t.Errorf("expected write of %q to be rejected", name)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(templateDir), "escape.html")); err == nil {
		t.Fatal("template written outside of the template folder")
	}

	if err := m.WriteTemplate("ok.html", []byte("x"), false); err != nil {
		t.Fatal(err)
	}
	if err := m.RenameTemplate("ok.html", "../escape.html"); err == nil {
		t.Error("expected rename to escape path to be rejected")
	}
	if err := m.DeleteTemplate("../ok.html"); err == nil {
		t.Error("expected delete with escape path to be rejected")
	}
}

func TestWriteRefusesSymlink(t *testing.T) {
	m, templateDir := newTestManager(t)
	outside := filepath.Join(t.TempDir(), "outside.html")
	os.WriteFile(outside, []byte("original"), 0644)
	os.MkdirAll(templateDir, 0775)
	if err := os.Symlink(outside, filepath.Join(templateDir, "link.html")); err != nil {
		t.Skip("symlink not supported:", err)
	}

	if err := m.WriteTemplate("link.html", []byte("overwritten"), false); err == nil {
		t.Error("expected write through symlink to be rejected")
	}
	if _, err := m.ReadTemplate("link.html"); err == nil {
		t.Error("expected read through symlink to be rejected")
	}
	content, _ := os.ReadFile(outside)
	if string(content) != "original" {
		t.Fatal("symlink target was modified")
	}
}

func TestTemplateCRUD(t *testing.T) {
	m, _ := newTestManager(t)

	if err := m.WriteTemplate("custom.html", []byte("hello"), true); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteTemplate("custom.html", []byte("again"), true); err != ErrTemplateExists {
		t.Errorf("expected ErrTemplateExists, got %v", err)
	}
	if err := m.WriteTemplate("custom.html", []byte("updated"), false); err != nil {
		t.Fatal(err)
	}
	content, err := m.ReadTemplate("custom.html")
	if err != nil || string(content) != "updated" {
		t.Fatalf("unexpected content %q, %v", content, err)
	}

	if err := m.SetAssignment(ActionWhitelist, "custom.html"); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteTemplate("other.html", []byte("other"), true); err != nil {
		t.Fatal(err)
	}
	if err := m.RenameTemplate("custom.html", "other.html"); err != ErrTemplateExists {
		t.Errorf("expected rename onto existing template to fail, got %v", err)
	}
	if err := m.RenameTemplate("custom.html", "renamed.html"); err != nil {
		t.Fatal(err)
	}
	if got := m.GetAssignment(ActionWhitelist); got != "renamed.html" {
		t.Errorf("assignment should follow rename, got %q", got)
	}

	if err := m.DeleteTemplate("renamed.html"); err != nil {
		t.Fatal(err)
	}
	if got := m.GetAssignment(ActionWhitelist); got != "" {
		t.Errorf("assignment should be cleared after delete, got %q", got)
	}

	if err := m.WriteTemplate("big.html", bytes.Repeat([]byte("a"), MaxTemplateSize+1), false); err != ErrTemplateTooLarge {
		t.Errorf("expected ErrTemplateTooLarge, got %v", err)
	}
	if err := m.SetAssignment(ActionWhitelist, "missing.html"); err == nil {
		t.Error("expected assigning a missing template to fail")
	}
	if err := m.SetAssignment("unknown", ""); err == nil {
		t.Error("expected assigning to an unknown action to fail")
	}
}

func TestResolveLookupOrder(t *testing.T) {
	m, _ := newTestManager(t)

	//Nothing exists, use built-in
	content, name := m.Resolve(ActionWhitelist, 403)
	if name != "" || !bytes.Equal(content, pageForbidden) {
		t.Fatalf("expected built-in template, got %q", name)
	}

	//Status code template
	m.WriteTemplate("403.html", []byte("code"), false)
	if _, name := m.Resolve(ActionWhitelist, 403); name != "403.html" {
		t.Errorf("expected 403.html, got %q", name)
	}

	//Legacy template wins over status code template
	m.WriteTemplate("whitelist.html", []byte("legacy"), false)
	if _, name := m.Resolve(ActionWhitelist, 403); name != "whitelist.html" {
		t.Errorf("expected whitelist.html, got %q", name)
	}

	//Assigned template wins over legacy template
	m.WriteTemplate("assigned.html", []byte("assigned"), false)
	m.SetAssignment(ActionWhitelist, "assigned.html")
	if _, name := m.Resolve(ActionWhitelist, 403); name != "assigned.html" {
		t.Errorf("expected assigned.html, got %q", name)
	}

	//Forced built-in
	m.SetAssignment(ActionWhitelist, AssignBuiltin)
	if _, name := m.Resolve(ActionWhitelist, 403); name != "" {
		t.Errorf("expected built-in, got %q", name)
	}

	//Vdir block uses the status code of the rule
	m.WriteTemplate("410.html", []byte("gone"), false)
	if _, name := m.Resolve(ActionVdirBlock, 410); name != "410.html" {
		t.Errorf("expected 410.html, got %q", name)
	}
	if content, name := m.Resolve(ActionVdirBlock, 451); name != "" || !bytes.Equal(content, pageGeneric) {
		t.Errorf("expected generic built-in, got %q", name)
	}

	//Assignment persists across manager reload
	m2 := NewManager(m.options)
	if got := m2.GetAssignment(ActionWhitelist); got != AssignBuiltin {
		t.Errorf("expected assignment to persist, got %q", got)
	}
}

func TestServeErrorPage(t *testing.T) {
	m, _ := newTestManager(t)
	m.WriteTemplate("418.html", []byte("<p>{{status_code}} {{status_text}}</p>"), false)

	rec := httptest.NewRecorder()
	m.ServeErrorPage(rec, ActionVdirBlock, 418)
	if rec.Code != 418 {
		t.Errorf("expected status 418, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "<p>418 I'm a teapot</p>" {
		t.Errorf("unexpected body %q", body)
	}

	//Nil manager serves built-in pages with the action default status code
	var nilManager *Manager
	rec = httptest.NewRecorder()
	nilManager.ServeErrorPage(rec, ActionRateLimit, 0)
	if rec.Code != 429 || !bytes.Contains(rec.Body.Bytes(), []byte("429")) {
		t.Errorf("unexpected nil manager response %d", rec.Code)
	}
}
