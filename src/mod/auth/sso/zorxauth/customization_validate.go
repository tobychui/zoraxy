package zorxauth

/*
	customization_validate.go

	Content validation for administrator uploaded login page resources.

	Uploaded resources are served from the authentication gateway origin, which
	is the origin holding the SSO session cookie, and from the webmin origin
	through the preview endpoint. A resource that can execute script on either
	origin is a session takeover, so every upload is checked to actually be the
	image format its extension claims, and SVGs are additionally screened for
	active content before they are ever written to disk.

	This is defence in depth: writeBrandingAsset() also serves every resource
	under a "default-src 'none'; sandbox" Content Security Policy so a payload
	that slips through the parser still cannot execute when navigated directly.
*/

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"image"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// svgForbiddenElements lists SVG elements that can execute script or embed a
// foreign document. Element names are compared lowercased and namespace free.
var svgForbiddenElements = map[string]bool{
	"script":        true, //Direct script execution
	"foreignobject": true, //Embeds arbitrary HTML, including <script>
	"iframe":        true,
	"embed":         true,
	"object":        true,
	"audio":         true,
	"video":         true,
	"handler":       true, //SVG Tiny event handler element
	"animate":       true, //Can animate an arbitrary attribute into an href/event handler
	"set":           true, //Same, via attributeName
}

// svgAllowedURLPrefixes are the only reference targets allowed in href / src
// style attributes. Same document fragments and inline data images are safe,
// remote references are not (they leak visitor IPs and can pull in script).
var svgAllowedURLPrefixes = []string{"#", "data:image/png", "data:image/jpeg", "data:image/gif", "data:image/webp"}

// validateUploadContent verifies that content really is the format claimed by
// ext, and that it is safe to serve. spec is used to enforce the aspect ratio
// of the default resource being replaced.
func validateUploadContent(content []byte, ext string, spec *AssetSpec) error {
	switch ext {
	case ".svg":
		return validateSVGContent(content)
	case ".png", ".jpg", ".jpeg", ".webp":
		return validateRasterContent(content, ext, spec)
	}
	return errors.New("Unsupported file type. Accepted formats: SVG, PNG, JPG, WEBP")
}

/* ── SVG ─────────────────────────────────────────────────────────────── */

// validateSVGContent rejects SVG uploads carrying active or external content.
func validateSVGContent(content []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	//Allow the named entities a design tool may emit, but see the Directive
	//check below: documents declaring their own entities are rejected outright.
	decoder.Entity = xml.HTMLEntity
	decoder.Strict = true

	sawRoot := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("SVG file is not well formed XML: " + err.Error())
		}

		switch t := token.(type) {
		case xml.ProcInst:
			//<?xml-stylesheet?> can pull in a remote stylesheet
			if !strings.EqualFold(t.Target, "xml") {
				return errors.New("SVG file contains an unsupported processing instruction: <?" + t.Target + "?>")
			}

		case xml.Directive:
			//Reject inline entity declarations (XXE and billion laughs)
			directive := strings.ToUpper(string(t))
			if strings.Contains(directive, "ENTITY") {
				return errors.New("SVG file declares XML entities, which are not allowed")
			}

		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if !sawRoot {
				if name != "svg" {
					return errors.New("SVG file root element must be <svg>")
				}
				sawRoot = true
			}

			if svgForbiddenElements[name] {
				return errors.New("SVG file contains a disallowed <" + name + "> element. Remove any scripts, animations or embedded documents from the artwork.")
			}

			if err := validateSVGAttributes(name, t.Attr); err != nil {
				return err
			}
		}
	}

	if !sawRoot {
		return errors.New("SVG file does not contain an <svg> element")
	}

	return nil
}

// validateSVGAttributes screens a single element's attributes for event
// handlers, script URLs and remote references.
func validateSVGAttributes(elementName string, attrs []xml.Attr) error {
	for _, attr := range attrs {
		name := strings.ToLower(attr.Name.Local)
		value := strings.TrimSpace(attr.Value)

		//Event handlers: onload, onclick, onmouseover, ...
		if strings.HasPrefix(name, "on") {
			return errors.New("SVG file contains an event handler attribute (" + attr.Name.Local + "). Remove any interactivity from the artwork.")
		}

		//Script URLs in any attribute, including style
		if containsScriptURL(value) {
			return errors.New("SVG file contains a script URL in the " + attr.Name.Local + " attribute")
		}

		//Reference attributes may only point inside the document or at inline image data
		if name == "href" || name == "src" {
			if !isAllowedSVGReference(value) {
				return errors.New("SVG file references an external resource (" + attr.Name.Local + "). Embed the artwork instead of linking to it.")
			}
		}

		//Inline CSS may not import remote stylesheets
		if name == "style" && strings.Contains(strings.ToLower(value), "@import") {
			return errors.New("SVG file imports an external stylesheet, which is not allowed")
		}
	}

	return nil
}

// containsScriptURL reports whether value carries a scheme that can execute code.
// Whitespace and control characters are stripped first because browsers ignore
// them when resolving a URL scheme (e.g. "java\nscript:alert(1)").
func containsScriptURL(value string) bool {
	var sb strings.Builder
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			continue
		}
		sb.WriteRune(unicode.ToLower(r))
	}
	normalized := sb.String()

	return strings.Contains(normalized, "javascript:") ||
		strings.Contains(normalized, "vbscript:") ||
		strings.Contains(normalized, "data:text/html") ||
		strings.Contains(normalized, "data:image/svg+xml")
}

// isAllowedSVGReference reports whether a href / src value is a same document
// fragment or an inline raster data URI.
func isAllowedSVGReference(value string) bool {
	if value == "" {
		return true
	}

	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, prefix := range svgAllowedURLPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}

	return false
}

/* ── Raster ──────────────────────────────────────────────────────────── */

// validateRasterContent verifies the file signature matches the extension, then
// enforces the aspect ratio of the resource being replaced.
func validateRasterContent(content []byte, ext string, spec *AssetSpec) error {
	if err := checkRasterSignature(content, ext); err != nil {
		return err
	}

	width, height, err := rasterDimensions(content, ext)
	if err != nil {
		return err
	}

	return checkAspectRatio(width, height, spec)
}

// checkRasterSignature rejects files whose magic bytes do not match their
// extension, so arbitrary content cannot be served under an image content type.
func checkRasterSignature(content []byte, ext string) error {
	valid := false
	switch ext {
	case ".png":
		valid = bytes.HasPrefix(content, []byte("\x89PNG\r\n\x1a\n"))
	case ".jpg", ".jpeg":
		valid = bytes.HasPrefix(content, []byte("\xff\xd8\xff"))
	case ".webp":
		valid = len(content) >= 12 && bytes.HasPrefix(content, []byte("RIFF")) && bytes.Equal(content[8:12], []byte("WEBP"))
	}

	if !valid {
		return errors.New("File content does not match its " + ext + " extension. The file may be corrupted or renamed.")
	}

	return nil
}

// rasterDimensions returns the pixel dimensions of a validated raster upload
func rasterDimensions(content []byte, ext string) (int, int, error) {
	if ext == ".webp" {
		return webpDimensions(content)
	}

	config, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return 0, 0, errors.New("Could not read the image dimensions. Try re-exporting the file as PNG.")
	}

	return config.Width, config.Height, nil
}

// webpDimensions reads the canvas size out of a WEBP container. The standard
// library has no WEBP decoder, so the three container variants are read here.
func webpDimensions(content []byte) (int, int, error) {
	//Chunks start after the 12 byte RIFF header: 4 byte FourCC + 4 byte size
	if len(content) < 30 {
		return 0, 0, errors.New("WEBP file is truncated")
	}

	switch string(content[12:16]) {
	case "VP8X":
		//Extended format: 24 bit little endian canvas width-1 / height-1 at offset 24
		width := int(content[24]) | int(content[25])<<8 | int(content[26])<<16
		height := int(content[27]) | int(content[28])<<8 | int(content[29])<<16
		return width + 1, height + 1, nil

	case "VP8 ":
		//Lossy: 3 byte start code 0x9d012a, then 16 bit width / height (14 bit each)
		if len(content) < 30 || content[23] != 0x9d || content[24] != 0x01 || content[25] != 0x2a {
			return 0, 0, errors.New("WEBP file has an unsupported lossy header")
		}
		width := int(binary.LittleEndian.Uint16(content[26:28]) & 0x3fff)
		height := int(binary.LittleEndian.Uint16(content[28:30]) & 0x3fff)
		return width, height, nil

	case "VP8L":
		//Lossless: signature byte 0x2f, then 14 bit width-1 and 14 bit height-1
		if content[20] != 0x2f {
			return 0, 0, errors.New("WEBP file has an unsupported lossless header")
		}
		bits := binary.LittleEndian.Uint32(content[21:25])
		width := int(bits&0x3fff) + 1
		height := int((bits>>14)&0x3fff) + 1
		return width, height, nil
	}

	return 0, 0, errors.New("Could not read the WEBP image dimensions. Try re-exporting the file as PNG.")
}

// checkAspectRatio rejects images whose width : height ratio differs from the
// default resource they replace, which would break the login page layout.
func checkAspectRatio(width int, height int, spec *AssetSpec) error {
	if width <= 0 || height <= 0 {
		return errors.New("Uploaded image has invalid dimensions")
	}

	expected := float64(spec.Width) / float64(spec.Height)
	actual := float64(width) / float64(height)
	if math.Abs(actual-expected)/expected > BRANDING_ASPECT_TOLERANCE {
		return errors.New("Image aspect ratio must match the default resource (" +
			strconv.Itoa(spec.Width) + " × " + strconv.Itoa(spec.Height) + "). Uploaded image is " +
			strconv.Itoa(width) + " × " + strconv.Itoa(height) +
			". Use the scaling tool to adjust it before uploading.")
	}

	return nil
}

/* ── Text ────────────────────────────────────────────────────────────── */

// sanitizeBrandingText strips control characters from an administrator supplied
// label. The value is HTML escaped again at render time; this only keeps the
// stored value to a single printable line.
func sanitizeBrandingText(value string) string {
	var sb strings.Builder
	for _, r := range value {
		if r == '\t' || r == '\n' || r == '\r' {
			sb.WriteRune(' ')
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		sb.WriteRune(r)
	}

	return strings.TrimSpace(sb.String())
}
