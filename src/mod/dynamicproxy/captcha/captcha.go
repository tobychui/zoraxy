package captcha

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

/*
	captcha.go

	CAPTCHA verification and session management for gating access to endpoints.
	Supports Cloudflare Turnstile and Google reCAPTCHA (v2 and v3).
*/

//go:embed cloudflare_turnstile.html
var cloudflareTurnstileTemplate string

//go:embed google_recaptcha_v2.html
var googleRecaptchaV2Template string

//go:embed google_recaptcha_v3.html
var googleRecaptchaV3Template string

const (
	CookieName             = "zoraxy_captcha_session"
	VerifyPath             = "/.zoraxy/captcha/verify"
	DefaultSessionDuration = 3600 // 1 hour in seconds
	CloudflareTurnstileAPI = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	GoogleRecaptchaAPIv2v3 = "https://www.google.com/recaptcha/api/siteverify"
)

// Provider represents the CAPTCHA provider type
type Provider int

const (
	ProviderCloudflare Provider = 0
	ProviderGoogle     Provider = 1
)

// ExceptionType defines the type of CAPTCHA exception rule
type ExceptionType int

const (
	ExceptionTypePaths ExceptionType = 0x00
	ExceptionTypeCIDR  ExceptionType = 0x01
)

// Config holds the configuration for CAPTCHA
type Config struct {
	Provider         Provider         `json:"Provider"`
	SiteKey          string           `json:"SiteKey"`
	SecretKey        string           `json:"SecretKey"`
	SessionDuration  int              `json:"SessionDuration"`
	RecaptchaVersion string           `json:"RecaptchaVersion"` // v2 or v3
	RecaptchaScore   float64          `json:"RecaptchaScore"`   // v3 only, 0.0-1.0
	ExceptionRules   []*ExceptionRule `json:"ExceptionRules"`
}

// ExceptionRule defines rules for bypassing CAPTCHA
type ExceptionRule struct {
	RuleType   ExceptionType `json:"RuleType"`
	PathPrefix string        `json:"PathPrefix"` // For path-based exceptions
	CIDR       string        `json:"CIDR"`       // For IP-based exceptions
}

// SessionStore manages active CAPTCHA sessions
type SessionStore struct {
	sessions sync.Map // map[sessionID]expiryTime
	stopChan chan struct{}
}

// NewSessionStore creates a new CAPTCHA session store
func NewSessionStore() *SessionStore {
	store := &SessionStore{
		stopChan: make(chan struct{}),
	}
	go store.cleanupExpiredSessions()
	return store
}

// generateSessionID creates a random session ID
func generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// AddSession adds a new session with the specified duration
func (s *SessionStore) AddSession(sessionID string, durationSeconds int) {
	expiryTime := time.Now().Add(time.Duration(durationSeconds) * time.Second)
	s.sessions.Store(sessionID, expiryTime)
}

// IsValidSession checks if a session is valid and not expired
func (s *SessionStore) IsValidSession(sessionID string) bool {
	value, exists := s.sessions.Load(sessionID)
	if !exists {
		return false
	}
	expiryTime, ok := value.(time.Time)
	if !ok {
		return false
	}
	return time.Now().Before(expiryTime)
}

// RemoveSession removes a session from the store
func (s *SessionStore) RemoveSession(sessionID string) {
	s.sessions.Delete(sessionID)
}

// Close stops the cleanup goroutine and releases resources
func (s *SessionStore) Close() {
	close(s.stopChan)
}

// cleanupExpiredSessions periodically removes expired sessions
func (s *SessionStore) cleanupExpiredSessions() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			s.sessions.Range(func(key, value interface{}) bool {
				expiryTime, ok := value.(time.Time)
				if !ok || now.After(expiryTime) {
					s.sessions.Delete(key)
				}
				return true
			})
		case <-s.stopChan:
			return
		}
	}
}

// CloudflareTurnstileResponse represents the response from Cloudflare Turnstile API
type CloudflareTurnstileResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes"`
	Action      string   `json:"action"`
	CData       string   `json:"cdata"`
}

// GoogleRecaptchaResponse represents the response from Google reCAPTCHA API
type GoogleRecaptchaResponse struct {
	Success     bool     `json:"success"`
	Score       float64  `json:"score"`        // v3 only
	Action      string   `json:"action"`       // v3 only
	ChallengeTS string   `json:"challenge_ts"` // ISO timestamp
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes"`
}

// VerifyCloudflareToken verifies a Cloudflare Turnstile token
func VerifyCloudflareToken(token, secretKey, remoteIP string) (bool, error) {
	if token == "" || secretKey == "" {
		return false, errors.New("token and secret key are required")
	}

	formData := url.Values{
		"secret":   {secretKey},
		"response": {token},
	}
	if remoteIP != "" {
		formData.Add("remoteip", remoteIP)
	}

	resp, err := http.PostForm(CloudflareTurnstileAPI, formData)
	if err != nil {
		return false, fmt.Errorf("failed to verify token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	var result CloudflareTurnstileResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return false, fmt.Errorf("verification failed: %v", result.ErrorCodes)
	}

	return true, nil
}

// VerifyGoogleRecaptchaToken verifies a Google reCAPTCHA token
func VerifyGoogleRecaptchaToken(token, secretKey, remoteIP string, version string, minScore float64) (bool, error) {
	if token == "" || secretKey == "" {
		return false, errors.New("token and secret key are required")
	}

	formData := url.Values{
		"secret":   {secretKey},
		"response": {token},
	}
	if remoteIP != "" {
		formData.Add("remoteip", remoteIP)
	}

	resp, err := http.PostForm(GoogleRecaptchaAPIv2v3, formData)
	if err != nil {
		return false, fmt.Errorf("failed to verify token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	var result GoogleRecaptchaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return false, fmt.Errorf("verification failed: %v", result.ErrorCodes)
	}

	// For v3, check the score
	if version == "v3" {
		if result.Score < minScore {
			return false, fmt.Errorf("score too low: %f < %f", result.Score, minScore)
		}
	}

	return true, nil
}

// GetClientIP extracts the real client IP from the request
func GetClientIP(r *http.Request) string {
	// Check X-Real-IP header first
	if ip := r.Header.Get("X-Real-Ip"); ip != "" {
		return ip
	}

	// Check CF-Connecting-IP for Cloudflare
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}

	// Check Fastly-Client-IP
	if ip := r.Header.Get("Fastly-Client-IP"); ip != "" {
		return ip
	}

	// Check X-Forwarded-For
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// CheckException checks if the request matches any exception rules
func CheckException(r *http.Request, rules []*ExceptionRule) bool {
	if rules == nil || len(rules) == 0 {
		return false
	}

	clientIP := GetClientIP(r)
	requestPath := r.URL.Path

	for _, rule := range rules {
		switch rule.RuleType {
		case ExceptionTypePaths:
			if strings.HasPrefix(requestPath, rule.PathPrefix) {
				return true
			}
		case ExceptionTypeCIDR:
			if rule.CIDR != "" {
				// Check if it's a single IP or CIDR
				if !strings.Contains(rule.CIDR, "/") {
					// Single IP
					if clientIP == rule.CIDR {
						return true
					}
				} else {
					// CIDR range
					_, ipNet, err := net.ParseCIDR(rule.CIDR)
					if err == nil {
						ip := net.ParseIP(clientIP)
						if ip != nil && ipNet.Contains(ip) {
							return true
						}
					}
				}
			}
		}
	}

	return false
}

// TemplateData holds the data for rendering CAPTCHA templates
type TemplateData struct {
	Domain     string
	SiteKey    string
	VerifyPath string
}

// loadExternalTemplate tries to load an external template from the filesystem
func loadExternalTemplate(webDir string, templateName string) (string, error) {
	// Try both .html and .tmpl extensions
	possiblePaths := []string{
		filepath.Join(webDir, "templates", templateName+".html"),
		filepath.Join(webDir, "templates", templateName+".tmpl"),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			// File exists, read it
			content, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("failed to read external template: %w", err)
			}
			return string(content), nil
		}
	}

	return "", os.ErrNotExist
}

// RenderChallenge renders the CAPTCHA challenge page
func RenderChallenge(w http.ResponseWriter, r *http.Request, config *Config, domain string, webDir string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)

	// Prepare template data
	data := TemplateData{
		Domain:     domain,
		SiteKey:    config.SiteKey,
		VerifyPath: VerifyPath,
	}

	var templateContent string
	var templateName string

	// Determine which template to use based on provider
	if config.Provider == ProviderCloudflare {
		templateName = "captcha_cloudflare"
		// Try to load external template first
		if webDir != "" {
			external, err := loadExternalTemplate(webDir, templateName)
			if err == nil {
				templateContent = external
			}
		}
		// Fall back to embedded template
		if templateContent == "" {
			templateContent = cloudflareTurnstileTemplate
		}
	} else {
		version := config.RecaptchaVersion
		if version == "" {
			version = "v2"
		}

		if version == "v2" {
			templateName = "captcha_recaptcha_v2"
			// Try to load external template first
			if webDir != "" {
				external, err := loadExternalTemplate(webDir, templateName)
				if err == nil {
					templateContent = external
				}
			}
			// Fall back to embedded template
			if templateContent == "" {
				templateContent = googleRecaptchaV2Template
			}
		} else {
			templateName = "captcha_recaptcha_v3"
			// Try to load external template first
			if webDir != "" {
				external, err := loadExternalTemplate(webDir, templateName)
				if err == nil {
					templateContent = external
				}
			}
			// Fall back to embedded template
			if templateContent == "" {
				templateContent = googleRecaptchaV3Template
			}
		}
	}

	// Parse and execute template
	tmpl, err := template.New(templateName).Parse(templateContent)
	if err != nil {
		http.Error(w, "Failed to parse template", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}

	w.Write(buf.Bytes())
}

// HandleVerification processes CAPTCHA verification requests
func HandleVerification(w http.ResponseWriter, r *http.Request, config *Config, sessionStore *SessionStore) error {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return errors.New("invalid method")
	}

	// Parse form data - handle both regular forms and multipart forms
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(1 << 20); err != nil { // 1 MB max
			http.Error(w, "Bad request", http.StatusBadRequest)
			return err
		}
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return err
		}
	}

	// Try to get token from POST form first, then from regular form
	token := r.PostFormValue("cf-turnstile-response")
	token = strings.TrimSpace(token)
	if token == "" {
		token = r.PostFormValue("g-recaptcha-response")
		token = strings.TrimSpace(token)
	}
	if token == "" {
		token = r.FormValue("cf-turnstile-response")
		token = strings.TrimSpace(token)
	}
	if token == "" {
		token = r.FormValue("g-recaptcha-response")
		token = strings.TrimSpace(token)
	}

	if token == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "CAPTCHA token missing",
		})
		return errors.New("token missing")
	}

	clientIP := GetClientIP(r)
	var verified bool
	var verifyErr error

	// Verify based on provider
	switch config.Provider {
	case ProviderCloudflare:
		verified, verifyErr = VerifyCloudflareToken(token, config.SecretKey, clientIP)
	case ProviderGoogle:
		version := config.RecaptchaVersion
		if version == "" {
			version = "v2"
		}
		minScore := config.RecaptchaScore
		if minScore == 0 {
			minScore = 0.5
		}
		verified, verifyErr = VerifyGoogleRecaptchaToken(token, config.SecretKey, clientIP, version, minScore)
	default:
		http.Error(w, "Invalid CAPTCHA provider", http.StatusInternalServerError)
		return errors.New("invalid provider")
	}

	if verifyErr != nil || !verified {
		// Verification failed
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		errorMsg := "CAPTCHA verification failed"
		if verifyErr != nil {
			errorMsg = verifyErr.Error()
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   errorMsg,
		})
		return fmt.Errorf("verification failed: %v", errorMsg)
	}

	// Create session
	sessionID, err := generateSessionID()
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return err
	}

	duration := config.SessionDuration
	if duration == 0 {
		duration = DefaultSessionDuration
	}
	sessionStore.AddSession(sessionID, duration)

	// Set cookie
	cookie := &http.Cookie{
		Name:     CookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   duration,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})

	// Return error to stop request chain (response already written)
	return errors.New("response already written - captcha verified")
}

// CheckSession checks if a request has a valid CAPTCHA session
func CheckSession(r *http.Request, sessionStore *SessionStore) bool {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return false
	}
	return sessionStore.IsValidSession(cookie.Value)
}

// IsConfigured checks if the CAPTCHA config is properly set up
func (config *Config) IsConfigured() bool {
	if config == nil {
		return false
	}
	if config.SiteKey == "" || config.SecretKey == "" {
		return false
	}
	return true
}
