package zorxauth

/*
	customization.go

	This file handles the branding customization of the Zoraxy Auth login page.

	Administrators can replace the four visual resources of the login page
	(brand mark, brand text, watermark icon / text and the panel art wallpaper)
	with their own. Uploaded resources are stored as plain files under
	./conf/sso/zorxauth/branding/ and the accompanying text settings are kept
	in branding.json inside the same folder.

	Resolution order at serve time is always:
		user uploaded resource  ->  go:embed default resource
*/

import (
	_ "embed"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	//Register the image decoders used for dimension validation
	_ "image/jpeg"
	_ "image/png"

	"imuslab.com/zoraxy/mod/utils"
)

//go:embed brand_mark.svg
var defaultBrandMarkSVG []byte

const (
	BRANDING_FOLDER      = "branding"
	BRANDING_CONFIG_FILE = "branding.json"

	BRANDING_MAX_UPLOAD_SIZE = 8 << 20 //8 MB, generous enough for a high resolution wallpaper
	//Slack over the file size limit for multipart part headers and boundaries
	BRANDING_MULTIPART_OVERHEAD = 1 << 16

	//Asset keys. These are also used as the uploaded file base names.
	ASSET_BRAND_MARK = "brandmark"
	ASSET_WATERMARK  = "watermark"
	ASSET_WALLPAPER  = "wallpaper"

	//Aspect ratio of an uploaded raster image may deviate from the default
	//resource by at most this ratio before it is rejected.
	BRANDING_ASPECT_TOLERANCE = 0.02
)

// AssetSpec describes the dimensions of a default (go:embed) resource.
// Uploaded replacements must keep the same width : height ratio.
type AssetSpec struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Width       int    `json:"width"`  //Recommended width in px
	Height      int    `json:"height"` //Recommended height in px
	Description string `json:"description"`
}

// assetSpecs is the source of truth for the recommended resolution of each
// customizable resource. The webmin UI reads this to lock its scaling tool to
// the correct aspect ratio.
var assetSpecs = map[string]*AssetSpec{
	ASSET_BRAND_MARK: {
		Key:         ASSET_BRAND_MARK,
		Label:       "Brand Mark",
		Width:       128,
		Height:      128,
		Description: "Square logo shown at the top left corner of the login page.",
	},
	ASSET_WATERMARK: {
		Key:         ASSET_WATERMARK,
		Label:       "Watermark Icon",
		Width:       600,
		Height:      200,
		Description: "Wide logo overlaid on the panel art. Use a transparent background and light colors.",
	},
	ASSET_WALLPAPER: {
		Key:         ASSET_WALLPAPER,
		Label:       "Panel Art Wallpaper",
		Width:       1200,
		Height:      1600,
		Description: "Portrait artwork filling the right hand panel. It is cropped to cover, so keep the focus centered.",
	},
}

// brandingUploadTypes maps an accepted file extension to its content type
var brandingUploadTypes = map[string]string{
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
}

// BrandingConfig holds the login page customization settings.
// An empty asset file name means the go:embed default is used.
type BrandingConfig struct {
	BrandText         string `json:"brandText"`         //Text next to the brand mark
	WatermarkText     string `json:"watermarkText"`     //Text under the watermark icon on the panel art
	HideWatermarkText bool   `json:"hideWatermarkText"` //Omit the watermark text from the login page
	HideWatermarkIcon bool   `json:"hideWatermarkIcon"` //Omit the watermark icon from the login page
	BrandMarkFile     string `json:"brandMarkFile"`     //File name of the uploaded brand mark, relative to the branding folder
	WatermarkFile     string `json:"watermarkFile"`     //File name of the uploaded watermark icon
	WallpaperFile     string `json:"wallpaperFile"`     //File name of the uploaded panel art wallpaper
	Revision          int64  `json:"revision"`          //Bumped on every change, used as the asset cache buster
}

func getDefaultBranding() *BrandingConfig {
	return &BrandingConfig{
		BrandText:     "Zoraxy Auth",
		WatermarkText: "Authentication Gateway",
	}
}

/* ── Storage ─────────────────────────────────────────────────────────── */

// brandingFolder returns the path to the branding resource storage folder
func (ar *AuthRouter) brandingFolder() string {
	base := ar.Options.ConfigFolderPath
	if base == "" {
		base = "./conf/sso/zorxauth"
	}
	return filepath.Join(base, BRANDING_FOLDER)
}

// brandingConfigPath returns the path to branding.json
func (ar *AuthRouter) brandingConfigPath() string {
	return filepath.Join(ar.brandingFolder(), BRANDING_CONFIG_FILE)
}

// initBrandingStore ensures the storage folder exists and loads branding.json into memory
func (ar *AuthRouter) initBrandingStore() {
	ar.brandingMutex.Lock()
	defer ar.brandingMutex.Unlock()

	ar.branding = getDefaultBranding()

	folder := ar.brandingFolder()
	if err := os.MkdirAll(folder, 0755); err != nil {
		if ar.Logger != nil {
			ar.Logger.PrintAndLog("zorxauth", "Failed to create branding folder: "+err.Error(), err)
		}
		return
	}

	data, err := os.ReadFile(ar.brandingConfigPath())
	if err != nil {
		//No customization saved yet, defaults are already in place
		return
	}

	loaded := BrandingConfig{}
	if err := json.Unmarshal(data, &loaded); err != nil {
		if ar.Logger != nil {
			ar.Logger.PrintAndLog("zorxauth", "Failed to parse branding config, using defaults: "+err.Error(), err)
		}
		return
	}

	//Drop references to resources that no longer exist on disk so the page
	//falls back to the embedded defaults instead of serving a 404.
	loaded.BrandMarkFile = ar.validateBrandingFile(loaded.BrandMarkFile)
	loaded.WatermarkFile = ar.validateBrandingFile(loaded.WatermarkFile)
	loaded.WallpaperFile = ar.validateBrandingFile(loaded.WallpaperFile)

	if loaded.BrandText == "" {
		loaded.BrandText = getDefaultBranding().BrandText
	}
	if loaded.WatermarkText == "" {
		loaded.WatermarkText = getDefaultBranding().WatermarkText
	}

	ar.branding = &loaded
}

// validateBrandingFile returns filename if the file exists in the branding folder, empty string otherwise
func (ar *AuthRouter) validateBrandingFile(filename string) string {
	if filename == "" {
		return ""
	}
	//Never trust a path from disk, only accept a bare file name
	filename = filepath.Base(filename)
	if _, err := os.Stat(filepath.Join(ar.brandingFolder(), filename)); err != nil {
		return ""
	}
	return filename
}

// GetBranding returns a copy of the current branding configuration
func (ar *AuthRouter) GetBranding() BrandingConfig {
	ar.brandingMutex.RLock()
	defer ar.brandingMutex.RUnlock()

	if ar.branding == nil {
		return *getDefaultBranding()
	}
	return *ar.branding
}

// saveBranding persists the in-memory branding config to disk. Caller must hold the write lock.
func (ar *AuthRouter) saveBranding() error {
	folder := ar.brandingFolder()
	if err := os.MkdirAll(folder, 0755); err != nil {
		return err
	}

	ar.branding.Revision = time.Now().Unix()

	data, err := json.MarshalIndent(ar.branding, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(ar.brandingConfigPath(), data, 0644)
}

// brandingAssetFile returns the stored file name for the given asset key, or empty when the default is in use
func (b *BrandingConfig) brandingAssetFile(assetKey string) string {
	switch assetKey {
	case ASSET_BRAND_MARK:
		return b.BrandMarkFile
	case ASSET_WATERMARK:
		return b.WatermarkFile
	case ASSET_WALLPAPER:
		return b.WallpaperFile
	}
	return ""
}

// setBrandingAssetFile updates the stored file name for the given asset key
func (b *BrandingConfig) setBrandingAssetFile(assetKey string, filename string) {
	switch assetKey {
	case ASSET_BRAND_MARK:
		b.BrandMarkFile = filename
	case ASSET_WATERMARK:
		b.WatermarkFile = filename
	case ASSET_WALLPAPER:
		b.WallpaperFile = filename
	}
}

/* ── Serving ─────────────────────────────────────────────────────────── */

// defaultAssetContent returns the go:embed fallback for an asset key
func defaultAssetContent(assetKey string) ([]byte, string) {
	switch assetKey {
	case ASSET_BRAND_MARK:
		return defaultBrandMarkSVG, "image/svg+xml"
	case ASSET_WATERMARK:
		return logoWhiteSVG, "image/svg+xml"
	case ASSET_WALLPAPER:
		return wallpaperSVG, "image/svg+xml"
	}
	return nil, ""
}

// ServeBrandingAsset writes the customized resource for assetKey, falling back
// to the embedded default when the administrator has not uploaded one.
func (ar *AuthRouter) ServeBrandingAsset(w http.ResponseWriter, r *http.Request, assetKey string) {
	branding := ar.GetBranding()

	if filename := branding.brandingAssetFile(assetKey); filename != "" {
		path := filepath.Join(ar.brandingFolder(), filepath.Base(filename))
		if content, err := os.ReadFile(path); err == nil {
			contentType := brandingUploadTypes[strings.ToLower(filepath.Ext(filename))]
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			writeBrandingAsset(w, content, contentType)
			return
		}
		//Unreadable upload, fall through to the embedded default
	}

	content, contentType := defaultAssetContent(assetKey)
	if content == nil {
		http.NotFound(w, r)
		return
	}
	writeBrandingAsset(w, content, contentType)
}

func writeBrandingAsset(w http.ResponseWriter, content []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	//Assets are addressed with a ?v= revision so they can be cached aggressively
	w.Header().Set("Cache-Control", "public, max-age=86400")
	//Uploaded resources are only ever referenced as images, block sniffing them into anything else
	w.Header().Set("X-Content-Type-Options", "nosniff")
	//Defence in depth on top of the upload validation: an SVG navigated to
	//directly renders in a sandboxed unique origin with scripts and every
	//external fetch blocked, so it can never reach the SSO session origin.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox")
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

// RenderAuthPage fills the branding placeholders of the login page template
func (ar *AuthRouter) RenderAuthPage() []byte {
	branding := ar.GetBranding()
	revision := strconv.FormatInt(branding.Revision, 10)

	//Custom brand marks are served as an <img>, the default one is inlined so
	//it can pick up the page's light / dark theme colors through CSS variables.
	brandMark := string(defaultBrandMarkSVG)
	if branding.BrandMarkFile != "" {
		brandMark = `<img class="brand-mark" src="` + BRANDING_ASSET_PATH + ASSET_BRAND_MARK + `?v=` + revision + `" alt="">`
	}

	//Disabled watermark elements are left out of the markup entirely instead of
	//being hidden with CSS, so nothing is downloaded for them either.
	watermarkIcon := ""
	if !branding.HideWatermarkIcon {
		watermarkIcon = `<img src="` + BRANDING_ASSET_PATH + ASSET_WATERMARK + `?v=` + revision + `" alt="">`
	}

	watermarkLabel := ""
	if !branding.HideWatermarkText {
		watermarkLabel = `<span>` + html.EscapeString(branding.WatermarkText) + `</span>`
	}

	//Single pass replacement: a Replacer never re-scans what it just inserted,
	//so an administrator supplied label containing a placeholder such as
	//"{{brand_mark}}" is emitted literally instead of being expanded.
	replacer := strings.NewReplacer(
		"{{brand_mark}}", brandMark,
		"{{brand_text}}", html.EscapeString(branding.BrandText),
		"{{watermark_icon}}", watermarkIcon,
		"{{watermark_label}}", watermarkLabel,
		"{{wallpaper_src}}", BRANDING_ASSET_PATH+ASSET_WALLPAPER+"?v="+revision,
	)

	return []byte(replacer.Replace(string(authPageHTML)))
}

/* ── Webmin API ──────────────────────────────────────────────────────── */

// HandleCustomizationSettings handles reading and updating the text based branding settings
func (ar *AuthRouter) HandleCustomizationSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ar.handleCustomizationGET(w, r)
	case http.MethodPost:
		ar.handleCustomizationPOST(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (ar *AuthRouter) handleCustomizationGET(w http.ResponseWriter, r *http.Request) {
	branding := ar.GetBranding()
	defaults := getDefaultBranding()

	assets := []map[string]interface{}{}
	for _, key := range []string{ASSET_BRAND_MARK, ASSET_WATERMARK, ASSET_WALLPAPER} {
		spec := assetSpecs[key]
		assets = append(assets, map[string]interface{}{
			"key":         spec.Key,
			"label":       spec.Label,
			"width":       spec.Width,
			"height":      spec.Height,
			"description": spec.Description,
			"customized":  branding.brandingAssetFile(key) != "",
			"filename":    branding.brandingAssetFile(key),
		})
	}

	js, _ := json.Marshal(map[string]interface{}{
		"brandText":            branding.BrandText,
		"watermarkText":        branding.WatermarkText,
		"hideWatermarkText":    branding.HideWatermarkText,
		"hideWatermarkIcon":    branding.HideWatermarkIcon,
		"defaultBrandText":     defaults.BrandText,
		"defaultWatermarkText": defaults.WatermarkText,
		"revision":             branding.Revision,
		"assetPath":            BRANDING_ASSET_PATH,
		"gatewayPort":          ar.Options.GatewayPort,
		"assets":               assets,
	})

	utils.SendJSONResponse(w, string(js))
}

func (ar *AuthRouter) handleCustomizationPOST(w http.ResponseWriter, r *http.Request) {
	brandText, _ := utils.PostPara(r, "brandText")
	watermarkText, _ := utils.PostPara(r, "watermarkText")
	hideWatermarkText, _ := utils.PostBool(r, "hideWatermarkText")
	hideWatermarkIcon, _ := utils.PostBool(r, "hideWatermarkIcon")

	//Strip control characters so the stored label stays a single printable line.
	//It is HTML escaped again when the login page is rendered.
	brandText = sanitizeBrandingText(brandText)
	watermarkText = sanitizeBrandingText(watermarkText)

	if brandText == "" {
		brandText = getDefaultBranding().BrandText
	}
	if watermarkText == "" {
		watermarkText = getDefaultBranding().WatermarkText
	}

	//Keep the login page layout intact, these are single line labels
	if len([]rune(brandText)) > 48 || len([]rune(watermarkText)) > 48 {
		utils.SendErrorResponse(w, "Brand text and watermark text must be 48 characters or shorter")
		return
	}

	ar.brandingMutex.Lock()
	defer ar.brandingMutex.Unlock()

	ar.branding.BrandText = brandText
	ar.branding.WatermarkText = watermarkText
	ar.branding.HideWatermarkText = hideWatermarkText
	ar.branding.HideWatermarkIcon = hideWatermarkIcon

	if err := ar.saveBranding(); err != nil {
		utils.SendErrorResponse(w, "Failed to save customization settings: "+err.Error())
		return
	}

	utils.SendOK(w)
}

// HandleCustomizationPreview serves the currently effective resource for an asset
// so the webmin UI can preview it without depending on the gateway being reachable.
func (ar *AuthRouter) HandleCustomizationPreview(w http.ResponseWriter, r *http.Request) {
	assetKey, err := utils.GetPara(r, "asset")
	if err != nil {
		http.Error(w, "Asset type not defined", http.StatusBadRequest)
		return
	}

	if _, ok := assetSpecs[assetKey]; !ok {
		http.NotFound(w, r)
		return
	}

	ar.ServeBrandingAsset(w, r, assetKey)
}

// HandleCustomizationUpload accepts a replacement resource for one of the customizable assets
func (ar *AuthRouter) HandleCustomizationUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	//ParseMultipartForm's argument is only an in-memory limit, anything larger
	//spills to a temp file unbounded. Cap the request body itself so an
	//oversized upload cannot fill the disk before it is inspected.
	r.Body = http.MaxBytesReader(w, r.Body, BRANDING_MAX_UPLOAD_SIZE+BRANDING_MULTIPART_OVERHEAD)
	if err := r.ParseMultipartForm(BRANDING_MAX_UPLOAD_SIZE); err != nil {
		utils.SendErrorResponse(w, "Upload too large or malformed. Maximum resource size is 8 MB.")
		return
	}
	defer r.MultipartForm.RemoveAll()

	assetKey := r.FormValue("asset")
	spec, ok := assetSpecs[assetKey]
	if !ok {
		utils.SendErrorResponse(w, "Invalid asset type")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		utils.SendErrorResponse(w, "No file uploaded")
		return
	}
	defer file.Close()

	//Only the extension is taken from the uploaded name, and only if it is one
	//of the accepted formats. The stored file name is built from the asset key
	//below, so a crafted upload name can never influence the write path.
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if _, accepted := brandingUploadTypes[ext]; !accepted {
		utils.SendErrorResponse(w, "Unsupported file type. Accepted formats: SVG, PNG, JPG, WEBP")
		return
	}

	content, err := io.ReadAll(io.LimitReader(file, BRANDING_MAX_UPLOAD_SIZE+1))
	if err != nil {
		utils.SendErrorResponse(w, "Failed to read uploaded file")
		return
	}
	if len(content) == 0 {
		utils.SendErrorResponse(w, "Uploaded file is empty")
		return
	}
	if len(content) > BRANDING_MAX_UPLOAD_SIZE {
		utils.SendErrorResponse(w, "Uploaded file exceeds the 8 MB limit")
		return
	}

	//Verify the file really is the format its extension claims, screen SVGs for
	//active content, and hold raster uploads to the aspect ratio of the
	//resource they replace. See customization_validate.go.
	if err := validateUploadContent(content, ext, spec); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	folder := ar.brandingFolder()
	if err := os.MkdirAll(folder, 0755); err != nil {
		utils.SendErrorResponse(w, "Failed to create branding folder: "+err.Error())
		return
	}

	//The file name is derived from the asset key, never from user input
	filename := assetKey + ext
	if err := os.WriteFile(filepath.Join(folder, filename), content, 0644); err != nil {
		utils.SendErrorResponse(w, "Failed to save uploaded file: "+err.Error())
		return
	}

	ar.brandingMutex.Lock()
	defer ar.brandingMutex.Unlock()

	//Remove the previous upload if it used a different extension
	if previous := ar.branding.brandingAssetFile(assetKey); previous != "" && previous != filename {
		os.Remove(filepath.Join(folder, filepath.Base(previous)))
	}

	ar.branding.setBrandingAssetFile(assetKey, filename)
	if err := ar.saveBranding(); err != nil {
		utils.SendErrorResponse(w, "Failed to save customization settings: "+err.Error())
		return
	}

	if ar.Logger != nil {
		ar.Logger.PrintAndLog("zorxauth", "Login page "+spec.Label+" replaced with "+filename, nil)
	}

	utils.SendOK(w)
}

// HandleCustomizationReset restores one asset, or the whole customization, back to the embedded defaults
func (ar *AuthRouter) HandleCustomizationReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	assetKey, err := utils.PostPara(r, "asset")
	if err != nil {
		utils.SendErrorResponse(w, "Asset type not defined")
		return
	}

	if _, ok := assetSpecs[assetKey]; !ok && assetKey != "all" {
		utils.SendErrorResponse(w, "Invalid asset type")
		return
	}

	ar.brandingMutex.Lock()
	defer ar.brandingMutex.Unlock()

	folder := ar.brandingFolder()
	targets := []string{assetKey}
	if assetKey == "all" {
		targets = []string{ASSET_BRAND_MARK, ASSET_WATERMARK, ASSET_WALLPAPER}
		defaults := getDefaultBranding()
		ar.branding.BrandText = defaults.BrandText
		ar.branding.WatermarkText = defaults.WatermarkText
		ar.branding.HideWatermarkText = defaults.HideWatermarkText
		ar.branding.HideWatermarkIcon = defaults.HideWatermarkIcon
	}

	for _, key := range targets {
		if filename := ar.branding.brandingAssetFile(key); filename != "" {
			os.Remove(filepath.Join(folder, filepath.Base(filename)))
			ar.branding.setBrandingAssetFile(key, "")
		}
	}

	if err := ar.saveBranding(); err != nil {
		utils.SendErrorResponse(w, "Failed to save customization settings: "+err.Error())
		return
	}

	utils.SendOK(w)
}

