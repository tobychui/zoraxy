# CAPTCHA Gating Feature for Zoraxy

This document describes the CAPTCHA gating feature that has been added to Zoraxy.

## Overview

The CAPTCHA gating feature allows you to protect your endpoints with CAPTCHA challenges, similar to Cloudflare Turnstile. Users must solve a CAPTCHA before they can access protected endpoints. This feature supports both Cloudflare Turnstile and Google reCAPTCHA (v2 and v3).

## Features

- **Per-endpoint configuration**: Just like rate limiting, CAPTCHA can be enabled/disabled per endpoint
- **Multiple provider support**:
  - Cloudflare Turnstile
  - Google reCAPTCHA v2 (checkbox)
  - Google reCAPTCHA v3 (invisible with score)
- **Session management**: Validated users receive a session cookie (configurable duration)
- **Exception rules**: Exclude specific paths or IP ranges from CAPTCHA challenges
- **Modern UI**: Responsive CAPTCHA challenge pages with gradient backgrounds

## Configuration

### Per-Endpoint Settings

When adding or editing a proxy endpoint, you can configure CAPTCHA with the following parameters:

| Parameter | Type | Description |
|-----------|------|-------------|
| `captcha` | boolean | Enable/disable CAPTCHA for this endpoint |
| `captchaProvider` | integer | Provider type: `0` = Cloudflare Turnstile, `1` = Google reCAPTCHA |
| `captchaSiteKey` | string | Site key (public key) from your CAPTCHA provider |
| `captchaSecretKey` | string | Secret key (private key) from your CAPTCHA provider |
| `captchaSessionDuration` | integer | Session duration in seconds (default: 3600) |
| `captchaRecaptchaVersion` | string | For Google: "v2" or "v3" (default: "v2") |
| `captchaRecaptchaScore` | float | For Google reCAPTCHA v3: minimum score 0.0-1.0 (default: 0.5) |

### Example API Call

```bash
curl -X POST http://localhost:8000/api/proxy/edit \
  -d "rootname=example.com" \
  -d "captcha=true" \
  -d "captchaProvider=0" \
  -d "captchaSiteKey=YOUR_SITE_KEY" \
  -d "captchaSecretKey=YOUR_SECRET_KEY" \
  -d "captchaSessionDuration=7200"
```

### Configuration Storage

CAPTCHA configuration is stored in the proxy endpoint configuration files (`.config` files in `conf/http_proxy/`). The configuration is persisted as part of the `ProxyEndpoint` struct in JSON format.

## How It Works

### Request Flow

1. **Request arrives** at protected endpoint
2. **Check exceptions**: If path or IP matches exception rules → allow
3. **Check session cookie**: If valid session exists → allow
4. **Serve CAPTCHA challenge**: Display CAPTCHA page
5. **User solves CAPTCHA**
6. **Verification**: Submit token to provider API
7. **Create session**: On success, set cookie and allow access
8. **Redirect**: User is redirected to original destination

### Middleware Chain Order

The CAPTCHA middleware is positioned in the request chain as follows:

1. Access Control (blacklist/whitelist)
2. Exploit Detection
3. **Rate Limiting**
4. **CAPTCHA Gating** ← Inserted here
5. Authentication (Basic Auth / SSO)
6. Proxy to upstream

This ensures CAPTCHA verification happens after rate limiting but before authentication.

## CAPTCHA Exception Rules

You can exclude certain paths or IP addresses from CAPTCHA challenges:

### Exception Types

1. **Path-based exceptions**: Match by path prefix
   ```json
   {
     "RuleType": 0,
     "PathPrefix": "/api/v1/"
   }
   ```

2. **IP-based exceptions**: Match by IP or CIDR range
   ```json
   {
     "RuleType": 1,
     "CIDR": "192.168.1.0/24"
   }
   ```

### Use Cases

- Exclude API endpoints from CAPTCHA
- Whitelist internal IP ranges
- Skip CAPTCHA for specific paths (e.g., `/health`, `/metrics`)

## Session Management

### Session Store

The CAPTCHA session store (`CaptchaSessionStore`) is a global component that:
- Stores session IDs with expiration times
- Uses `sync.Map` for thread-safe concurrent access
- Automatically cleans up expired sessions every 5 minutes

### Session Cookies

When a user successfully completes a CAPTCHA:
- A random 64-character session ID is generated
- A cookie named `zoraxy_captcha_session` is set
- Cookie attributes:
  - `HttpOnly`: Yes (prevents JavaScript access)
  - `Secure`: Yes if TLS is enabled
  - `SameSite`: Lax
  - `MaxAge`: Configurable (default 1 hour)

## Provider Setup

### Cloudflare Turnstile

1. Sign up at https://dash.cloudflare.com/
2. Navigate to Turnstile section
3. Create a new site
4. Copy the **Site Key** and **Secret Key**
5. Configure in Zoraxy with `captchaProvider=0`

### Google reCAPTCHA v2

1. Visit https://www.google.com/recaptcha/admin
2. Register a new site
3. Select reCAPTCHA v2 (checkbox)
4. Copy the **Site Key** and **Secret Key**
5. Configure in Zoraxy with:
   - `captchaProvider=1`
   - `captchaRecaptchaVersion=v2`

### Google reCAPTCHA v3

1. Visit https://www.google.com/recaptcha/admin
2. Register a new site
3. Select reCAPTCHA v3
4. Copy the **Site Key** and **Secret Key**
5. Configure in Zoraxy with:
   - `captchaProvider=1`
   - `captchaRecaptchaVersion=v3`
   - `captchaRecaptchaScore=0.5` (adjust as needed)

## Code Structure

### New Files

- `src/mod/dynamicproxy/captcha.go`: Core CAPTCHA verification and session management logic

### Modified Files

- `src/mod/dynamicproxy/typedef.go`:
  - Added `CaptchaConfig`, `CaptchaProvider`, `CaptchaExceptionRule` types
  - Added `RequireCaptcha` and `CaptchaConfig` fields to `ProxyEndpoint`
  - Added `captchaSessionStore` field to `Router`

- `src/mod/dynamicproxy/dynamicproxy.go`:
  - Initialize `CaptchaSessionStore` in `NewDynamicProxy()`
  - Added CAPTCHA middleware to port 80 HTTP handler

- `src/mod/dynamicproxy/Server.go`:
  - Added CAPTCHA middleware to main request chain

- `src/reverseproxy.go`:
  - Added CAPTCHA parameter parsing in `ReverseProxyHandleAddEndpoint()`
  - Added CAPTCHA parameter parsing in `ReverseProxyHandleEditEndpoint()`
  - Added CAPTCHA configuration to endpoint creation

### Key Functions

- `handleCaptchaRouting()`: Main middleware function
- `handleCaptchaVerification()`: Process CAPTCHA token verification
- `serveCaptchaChallenge()`: Render CAPTCHA challenge page
- `VerifyCloudflareToken()`: Verify Cloudflare Turnstile token
- `VerifyGoogleRecaptchaToken()`: Verify Google reCAPTCHA token
- `CheckCaptchaException()`: Check if request matches exception rules

## Security Considerations

1. **Secret Key Protection**: Store CAPTCHA secret keys securely. They are stored in config files - ensure proper file permissions.

2. **Session Security**: Session IDs are cryptographically random (32 bytes from `crypto/rand`).

3. **Cookie Security**: Cookies use HttpOnly and Secure flags when TLS is enabled.

4. **Rate Limiting**: CAPTCHA works in conjunction with rate limiting, not as a replacement.

5. **Score Thresholds**: For reCAPTCHA v3, adjust the score threshold based on your traffic patterns (0.5 is a good starting point).

## Logging

CAPTCHA-related events are logged with the following identifiers:
- `captcha-required`: User was served a CAPTCHA challenge (403 status)

## Testing

### Manual Testing Steps

1. **Enable CAPTCHA on an endpoint**
2. **Access the endpoint** → Should show CAPTCHA challenge
3. **Complete CAPTCHA** → Should create session and allow access
4. **Access again** → Should bypass CAPTCHA (session valid)
5. **Wait for session expiry** → Should show CAPTCHA again

### Test with curl

```bash
# First request - should return CAPTCHA HTML
curl -i http://your-domain.com/

# After solving CAPTCHA in browser, copy session cookie
# Second request with session cookie - should proxy normally
curl -i -H "Cookie: zoraxy_captcha_session=YOUR_SESSION_ID" http://your-domain.com/
```

## API Endpoints

The following API endpoints support CAPTCHA configuration:

- `POST /api/proxy/add`: Add new endpoint with CAPTCHA
- `POST /api/proxy/edit`: Edit existing endpoint CAPTCHA settings

## Future Enhancements

Potential improvements for future versions:

1. **Web UI Integration**: Add CAPTCHA settings to the web-based admin panel
2. **Additional Providers**: Support for hCaptcha, FriendlyCaptcha
3. **Exception Rule Management API**: Dedicated endpoints for managing exception rules
4. **Analytics**: Track CAPTCHA solve rates and bot detection statistics
5. **Custom Challenge Pages**: Allow custom HTML templates for CAPTCHA pages
6. **Distributed Sessions**: Support for session sharing across multiple Zoraxy instances

## Troubleshooting

### CAPTCHA not showing

- Check that `RequireCaptcha` is `true` in endpoint configuration
- Verify `CaptchaConfig` is not `nil`
- Check logs for any errors

### Verification failures

- Verify Site Key and Secret Key are correct
- Check network connectivity to CAPTCHA provider APIs
- Ensure client IP detection is working correctly

### Session not persisting

- Check cookie settings in browser
- Verify session duration configuration
- Ensure cookies are not being blocked by browser settings

## Credits

Implemented following the existing Zoraxy architecture patterns:
- Rate limiting implementation for reference
- Basic authentication exception rules for exception handling pattern
- Access control for IP filtering patterns

---

For questions or issues, please file a GitHub issue at https://github.com/tobychui/zoraxy/issues
