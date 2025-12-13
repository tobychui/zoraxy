package dynamicproxy

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

/*
	captcha.go

	CAPTCHA verification and session management for gating access to endpoints.
	Supports Cloudflare Turnstile and Google reCAPTCHA (v2 and v3).
*/

const (
	CaptchaCookieName       = "zoraxy_captcha_session"
	CaptchaVerifyPath       = "/__zoraxy_captcha_verify"
	DefaultSessionDuration  = 3600 // 1 hour in seconds
	CloudflareTurnstileAPI  = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	GoogleRecaptchaAPIv2v3  = "https://www.google.com/recaptcha/api/siteverify"
)

// CaptchaSessionStore manages active CAPTCHA sessions
type CaptchaSessionStore struct {
	sessions sync.Map // map[sessionID]expiryTime
}

// NewCaptchaSessionStore creates a new CAPTCHA session store
func NewCaptchaSessionStore() *CaptchaSessionStore {
	store := &CaptchaSessionStore{}
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
func (s *CaptchaSessionStore) AddSession(sessionID string, durationSeconds int) {
	expiryTime := time.Now().Add(time.Duration(durationSeconds) * time.Second)
	s.sessions.Store(sessionID, expiryTime)
}

// IsValidSession checks if a session is valid and not expired
func (s *CaptchaSessionStore) IsValidSession(sessionID string) bool {
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
func (s *CaptchaSessionStore) RemoveSession(sessionID string) {
	s.sessions.Delete(sessionID)
}

// cleanupExpiredSessions periodically removes expired sessions
func (s *CaptchaSessionStore) cleanupExpiredSessions() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		s.sessions.Range(func(key, value interface{}) bool {
			expiryTime, ok := value.(time.Time)
			if !ok || now.After(expiryTime) {
				s.sessions.Delete(key)
			}
			return true
		})
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

// CheckCaptchaException checks if the request matches any exception rules
func CheckCaptchaException(r *http.Request, rules []*CaptchaExceptionRule) bool {
	if rules == nil || len(rules) == 0 {
		return false
	}

	clientIP := GetClientIP(r)
	requestPath := r.URL.Path

	for _, rule := range rules {
		switch rule.RuleType {
		case CaptchaExceptionType_Paths:
			if strings.HasPrefix(requestPath, rule.PathPrefix) {
				return true
			}
		case CaptchaExceptionType_CIDR:
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

// handleCaptchaRouting handles CAPTCHA verification for a proxy endpoint
func (h *ProxyHandler) handleCaptchaRouting(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint, sessionStore *CaptchaSessionStore) error {
	// Check if CAPTCHA verification endpoint
	if r.URL.Path == CaptchaVerifyPath {
		return h.handleCaptchaVerification(w, r, pe, sessionStore)
	}

	// Check for exception rules
	if pe.CaptchaConfig != nil && CheckCaptchaException(r, pe.CaptchaConfig.ExceptionRules) {
		return nil // Allow passthrough
	}

	// Check for existing valid session
	cookie, err := r.Cookie(CaptchaCookieName)
	if err == nil && sessionStore.IsValidSession(cookie.Value) {
		return nil // Session is valid, allow passthrough
	}

	// No valid session, serve CAPTCHA challenge
	h.serveCaptchaChallenge(w, r, pe)
	return errors.New("captcha required")
}

// handleCaptchaVerification processes CAPTCHA verification requests
func (h *ProxyHandler) handleCaptchaVerification(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint, sessionStore *CaptchaSessionStore) error {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return errors.New("invalid method")
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return err
	}

	token := r.FormValue("cf-turnstile-response")
	if token == "" {
		token = r.FormValue("g-recaptcha-response")
	}
	if token == "" {
		http.Error(w, "CAPTCHA token missing", http.StatusBadRequest)
		return errors.New("token missing")
	}

	clientIP := GetClientIP(r)
	var verified bool
	var verifyErr error

	// Verify based on provider
	if pe.CaptchaConfig.Provider == CaptchaProviderCloudflare {
		verified, verifyErr = VerifyCloudflareToken(token, pe.CaptchaConfig.SecretKey, clientIP)
	} else if pe.CaptchaConfig.Provider == CaptchaProviderGoogle {
		version := pe.CaptchaConfig.RecaptchaVersion
		if version == "" {
			version = "v2"
		}
		minScore := pe.CaptchaConfig.RecaptchaScore
		if minScore == 0 {
			minScore = 0.5
		}
		verified, verifyErr = VerifyGoogleRecaptchaToken(token, pe.CaptchaConfig.SecretKey, clientIP, version, minScore)
	} else {
		http.Error(w, "Invalid CAPTCHA provider", http.StatusInternalServerError)
		return errors.New("invalid provider")
	}

	if verifyErr != nil || !verified {
		// Verification failed
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "CAPTCHA verification failed",
		})
		return errors.New("verification failed")
	}

	// Create session
	sessionID, err := generateSessionID()
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return err
	}

	duration := pe.CaptchaConfig.SessionDuration
	if duration == 0 {
		duration = DefaultSessionDuration
	}
	sessionStore.AddSession(sessionID, duration)

	// Set cookie
	cookie := &http.Cookie{
		Name:     CaptchaCookieName,
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
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})

	return nil
}

// serveCaptchaChallenge serves the CAPTCHA challenge page
func (h *ProxyHandler) serveCaptchaChallenge(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)

	var captchaHTML bytes.Buffer
	if pe.CaptchaConfig.Provider == CaptchaProviderCloudflare {
		// Cloudflare Turnstile
		captchaHTML.WriteString(fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Security Check Required</title>
    <script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
        }
        .container {
            background: white;
            padding: 40px;
            border-radius: 10px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.1);
            text-align: center;
            max-width: 400px;
        }
        h1 {
            color: #333;
            margin-bottom: 10px;
            font-size: 24px;
        }
        p {
            color: #666;
            margin-bottom: 30px;
        }
        .captcha-container {
            display: flex;
            justify-content: center;
            margin: 20px 0;
        }
        #status {
            margin-top: 20px;
            padding: 10px;
            border-radius: 5px;
            display: none;
        }
        .error {
            background-color: #fee;
            color: #c33;
            border: 1px solid #fcc;
        }
        .success {
            background-color: #efe;
            color: #3c3;
            border: 1px solid #cfc;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🛡️ Security Check Required</h1>
        <p>Please complete the security check below to access this page.</p>

        <form id="captchaForm">
            <div class="captcha-container">
                <div class="cf-turnstile" data-sitekey="%s" data-callback="onCaptchaSuccess"></div>
            </div>
            <div id="status"></div>
        </form>
    </div>

    <script>
        function onCaptchaSuccess(token) {
            const formData = new FormData();
            formData.append('cf-turnstile-response', token);

            fetch('%s', {
                method: 'POST',
                body: formData
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    const status = document.getElementById('status');
                    status.textContent = 'Verification successful! Redirecting...';
                    status.className = 'success';
                    status.style.display = 'block';
                    setTimeout(() => {
                        window.location.reload();
                    }, 1000);
                } else {
                    const status = document.getElementById('status');
                    status.textContent = 'Verification failed. Please try again.';
                    status.className = 'error';
                    status.style.display = 'block';
                }
            })
            .catch(error => {
                const status = document.getElementById('status');
                status.textContent = 'An error occurred. Please try again.';
                status.className = 'error';
                status.style.display = 'block';
            });
        }
    </script>
</body>
</html>`, pe.CaptchaConfig.SiteKey, CaptchaVerifyPath))
	} else {
		// Google reCAPTCHA
		version := pe.CaptchaConfig.RecaptchaVersion
		if version == "" {
			version = "v2"
		}

		if version == "v2" {
			captchaHTML.WriteString(fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Security Check Required</title>
    <script src="https://www.google.com/recaptcha/api.js" async defer></script>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
        }
        .container {
            background: white;
            padding: 40px;
            border-radius: 10px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.1);
            text-align: center;
            max-width: 400px;
        }
        h1 {
            color: #333;
            margin-bottom: 10px;
            font-size: 24px;
        }
        p {
            color: #666;
            margin-bottom: 30px;
        }
        .captcha-container {
            display: flex;
            justify-content: center;
            margin: 20px 0;
        }
        #status {
            margin-top: 20px;
            padding: 10px;
            border-radius: 5px;
            display: none;
        }
        .error {
            background-color: #fee;
            color: #c33;
            border: 1px solid #fcc;
        }
        .success {
            background-color: #efe;
            color: #3c3;
            border: 1px solid #cfc;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🛡️ Security Check Required</h1>
        <p>Please complete the security check below to access this page.</p>

        <form id="captchaForm" onsubmit="return handleSubmit(event)">
            <div class="captcha-container">
                <div class="g-recaptcha" data-sitekey="%s" data-callback="onCaptchaSuccess"></div>
            </div>
            <div id="status"></div>
        </form>
    </div>

    <script>
        function onCaptchaSuccess(token) {
            const formData = new FormData();
            formData.append('g-recaptcha-response', token);

            fetch('%s', {
                method: 'POST',
                body: formData
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    const status = document.getElementById('status');
                    status.textContent = 'Verification successful! Redirecting...';
                    status.className = 'success';
                    status.style.display = 'block';
                    setTimeout(() => {
                        window.location.reload();
                    }, 1000);
                } else {
                    const status = document.getElementById('status');
                    status.textContent = 'Verification failed. Please try again.';
                    status.className = 'error';
                    status.style.display = 'block';
                    grecaptcha.reset();
                }
            })
            .catch(error => {
                const status = document.getElementById('status');
                status.textContent = 'An error occurred. Please try again.';
                status.className = 'error';
                status.style.display = 'block';
                grecaptcha.reset();
            });
        }

        function handleSubmit(e) {
            e.preventDefault();
            return false;
        }
    </script>
</body>
</html>`, pe.CaptchaConfig.SiteKey, CaptchaVerifyPath))
		} else {
			// reCAPTCHA v3
			captchaHTML.WriteString(fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Security Check Required</title>
    <script src="https://www.google.com/recaptcha/api.js?render=%s"></script>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
        }
        .container {
            background: white;
            padding: 40px;
            border-radius: 10px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.1);
            text-align: center;
            max-width: 400px;
        }
        h1 {
            color: #333;
            margin-bottom: 10px;
            font-size: 24px;
        }
        p {
            color: #666;
            margin-bottom: 30px;
        }
        .loader {
            border: 4px solid #f3f3f3;
            border-top: 4px solid #667eea;
            border-radius: 50%%;
            width: 40px;
            height: 40px;
            animation: spin 1s linear infinite;
            margin: 20px auto;
        }
        @keyframes spin {
            0%% { transform: rotate(0deg); }
            100%% { transform: rotate(360deg); }
        }
        #status {
            margin-top: 20px;
            padding: 10px;
            border-radius: 5px;
        }
        .error {
            background-color: #fee;
            color: #c33;
            border: 1px solid #fcc;
        }
        .success {
            background-color: #efe;
            color: #3c3;
            border: 1px solid #cfc;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🛡️ Security Check</h1>
        <p>Verifying your connection...</p>
        <div class="loader"></div>
        <div id="status"></div>
    </div>

    <script>
        grecaptcha.ready(function() {
            grecaptcha.execute('%s', {action: 'access'}).then(function(token) {
                const formData = new FormData();
                formData.append('g-recaptcha-response', token);

                fetch('%s', {
                    method: 'POST',
                    body: formData
                })
                .then(response => response.json())
                .then(data => {
                    const status = document.getElementById('status');
                    if (data.success) {
                        status.textContent = 'Verification successful! Redirecting...';
                        status.className = 'success';
                        setTimeout(() => {
                            window.location.reload();
                        }, 1000);
                    } else {
                        status.textContent = 'Verification failed. Access denied.';
                        status.className = 'error';
                    }
                })
                .catch(error => {
                    const status = document.getElementById('status');
                    status.textContent = 'An error occurred. Please refresh the page.';
                    status.className = 'error';
                });
            });
        });
    </script>
</body>
</html>`, pe.CaptchaConfig.SiteKey, pe.CaptchaConfig.SiteKey, CaptchaVerifyPath))
		}
	}

	w.Write(captchaHTML.Bytes())
}
