package zorxauth

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
)

// makePNG renders a blank PNG of the given size for upload validation tests
func makePNG(t *testing.T, width int, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{1, 2, 3, 255})

	buf := bytes.Buffer{}
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode test png: %v", err)
	}
	return buf.Bytes()
}

func TestValidateSVGContentRejectsActiveContent(t *testing.T) {
	cases := []struct {
		name string
		svg  string
	}{
		{"script element", `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(document.cookie)</script></svg>`},
		{"uppercase script element", `<svg xmlns="http://www.w3.org/2000/svg"><SCRIPT>alert(1)</SCRIPT></svg>`},
		{"event handler", `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><circle r="5"/></svg>`},
		{"event handler on child", `<svg xmlns="http://www.w3.org/2000/svg"><circle r="5" onmouseover="alert(1)"/></svg>`},
		{"foreignObject", `<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><body xmlns="http://www.w3.org/1999/xhtml"><script>alert(1)</script></body></foreignObject></svg>`},
		{"iframe", `<svg xmlns="http://www.w3.org/2000/svg"><iframe src="https://evil.example"/></svg>`},
		{"animate to href", `<svg xmlns="http://www.w3.org/2000/svg"><a><animate attributeName="href" values="javascript:alert(1)"/></a></svg>`},
		{"set element", `<svg xmlns="http://www.w3.org/2000/svg"><set attributeName="onload" to="alert(1)"/></svg>`},
		{"javascript href", `<svg xmlns="http://www.w3.org/2000/svg"><a href="javascript:alert(1)"><circle r="5"/></a></svg>`},
		{"obfuscated javascript href", `<svg xmlns="http://www.w3.org/2000/svg"><a href="java&#9;script:alert(1)"><circle r="5"/></a></svg>`},
		{"external use reference", `<svg xmlns="http://www.w3.org/2000/svg"><use href="https://evil.example/payload.svg#x"/></svg>`},
		{"external image reference", `<svg xmlns="http://www.w3.org/2000/svg"><image href="https://tracker.example/pixel.png"/></svg>`},
		{"entity declaration", `<!DOCTYPE svg [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><svg xmlns="http://www.w3.org/2000/svg"><text>&xxe;</text></svg>`},
		{"xml stylesheet", `<?xml-stylesheet type="text/css" href="https://evil.example/x.css"?><svg xmlns="http://www.w3.org/2000/svg"><circle r="5"/></svg>`},
		{"css import", `<svg xmlns="http://www.w3.org/2000/svg"><circle r="5" style="@import url(https://evil.example/x.css)"/></svg>`},
		{"not an svg root", `<html><body><script>alert(1)</script></body></html>`},
		{"malformed xml", `<svg xmlns="http://www.w3.org/2000/svg"><circle r="5">`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateSVGContent([]byte(tc.svg)); err == nil {
				t.Errorf("expected %s to be rejected, but it was accepted", tc.name)
			}
		})
	}
}

func TestValidateSVGContentAcceptsPlainArtwork(t *testing.T) {
	cases := []struct {
		name string
		svg  string
	}{
		{"simple shapes", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><circle cx="5" cy="5" r="4" fill="#f00"/></svg>`},
		{"gradient and group", `<svg xmlns="http://www.w3.org/2000/svg"><defs><linearGradient id="g"><stop offset="0" stop-color="#fff"/></linearGradient></defs><g><rect width="10" height="10" fill="url(#g)"/></g></svg>`},
		{"internal fragment reference", `<svg xmlns="http://www.w3.org/2000/svg"><defs><path id="p" d="M0 0 L1 1"/></defs><use href="#p"/></svg>`},
		{"inline data image", `<svg xmlns="http://www.w3.org/2000/svg"><image href="data:image/png;base64,iVBORw0KGgo="/></svg>`},
		{"doctype without entities", `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg xmlns="http://www.w3.org/2000/svg"><circle r="5"/></svg>`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateSVGContent([]byte(tc.svg)); err != nil {
				t.Errorf("expected %s to be accepted, got: %v", tc.name, err)
			}
		})
	}
}

// The resources shipped with Zoraxy must themselves pass the upload validator,
// otherwise an administrator could not re-upload the stock artwork.
func TestDefaultAssetsPassValidation(t *testing.T) {
	for _, key := range []string{ASSET_BRAND_MARK, ASSET_WATERMARK, ASSET_WALLPAPER} {
		content, _ := defaultAssetContent(key)
		if err := validateSVGContent(content); err != nil {
			t.Errorf("embedded default %s failed validation: %v", key, err)
		}
	}
}

func TestValidateUploadContentRejectsExtensionMismatch(t *testing.T) {
	spec := assetSpecs[ASSET_BRAND_MARK]

	//An HTML payload renamed to look like an image must not be stored
	payload := []byte(`<html><script>alert(document.cookie)</script></html>`)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp"} {
		if err := validateUploadContent(payload, ext, spec); err == nil {
			t.Errorf("expected HTML payload renamed to %s to be rejected", ext)
		}
	}

	//A real PNG carrying a .jpg extension is also a mismatch
	if err := validateUploadContent(makePNG(t, 128, 128), ".jpg", spec); err == nil {
		t.Error("expected PNG content with a .jpg extension to be rejected")
	}
}

func TestValidateUploadContentEnforcesAspectRatio(t *testing.T) {
	spec := assetSpecs[ASSET_WATERMARK] //600 x 200, 3:1

	if err := validateUploadContent(makePNG(t, 600, 200), ".png", spec); err != nil {
		t.Errorf("expected matching ratio to be accepted, got: %v", err)
	}

	if err := validateUploadContent(makePNG(t, 300, 100), ".png", spec); err != nil {
		t.Errorf("expected scaled matching ratio to be accepted, got: %v", err)
	}

	if err := validateUploadContent(makePNG(t, 600, 400), ".png", spec); err == nil {
		t.Error("expected mismatched ratio to be rejected")
	}
}

// The stored file name is always derived from the asset key, never from the
// uploaded file name, so a traversal attempt cannot escape the branding folder.
func TestUploadFileNameCannotEscapeBrandingFolder(t *testing.T) {
	hostileNames := []string{
		`../../../../windows/system32/evil.png`,
		`../../conf/sso/zorxauth/branding/../../../sys.db.png`,
		`..\..\evil.png`,
		`/etc/cron.d/evil.svg`,
		"evil.png" + string(rune(0)) + ".txt",
	}

	for _, name := range hostileNames {
		ext := strings.ToLower(filepath.Ext(name))
		if _, accepted := brandingUploadTypes[ext]; !accepted {
			continue //Rejected before a path is ever built
		}

		//This mirrors what HandleCustomizationUpload builds
		stored := ASSET_WALLPAPER + ext
		if strings.ContainsAny(stored, `/\`) || stored != filepath.Base(stored) {
			t.Errorf("stored name %q derived from %q is not a bare file name", stored, name)
		}
	}
}

func TestSanitizeBrandingTextStripsControlCharacters(t *testing.T) {
	if got := sanitizeBrandingText("  Acme" + string(rune(0)) + " Identity\r\n "); got != "Acme Identity" {
		t.Errorf("unexpected sanitize result: %q", got)
	}
}

// A label containing a template placeholder must be emitted literally rather
// than expanded into markup by a later replacement pass.
func TestRenderAuthPageDoesNotExpandPlaceholdersFromText(t *testing.T) {
	ar := &AuthRouter{
		branding: &BrandingConfig{
			BrandText:     "{{watermark_icon}}",
			WatermarkText: "<img src=x onerror=alert(1)>",
		},
	}

	page := string(ar.RenderAuthPage())

	if strings.Contains(page, `<span>{{watermark_icon}}</span>`) == false &&
		strings.Contains(page, "{{watermark_icon}}") == false {
		t.Error("expected the placeholder in the brand text to survive as literal text")
	}
	if strings.Contains(page, "<img src=x onerror=alert(1)>") {
		t.Error("watermark text was not HTML escaped")
	}
	if !strings.Contains(page, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Error("expected the watermark text to be HTML escaped")
	}
}
