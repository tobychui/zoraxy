package oauth2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jellydator/ttlcache/v3"
	gooauth2 "golang.org/x/oauth2"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

const (
	// DefaultOAuth2ConfigCacheTime defines the default cache duration for OAuth2 configuration
	DefaultOAuth2ConfigCacheTime = 60 * time.Second

	DefaultTenantID = "default"

	oauth2TableName  = "oauth2"
	oauth2TenantsKey = "tenants"

	legacyWellKnownKey       = "oauth2WellKnownUrl"
	legacyServerURLKey       = "oauth2ServerUrl"
	legacyTokenURLKey        = "oauth2TokenUrl"
	legacyClientIDKey        = "oauth2ClientId"
	legacyClientSecretKey    = "oauth2ClientSecret"
	legacyUserInfoURLKey     = "oauth2UserInfoUrl"
	legacyCodeChallengeKey   = "oauth2CodeChallengeMethod"
	legacyScopesKey          = "oauth2Scopes"
	legacyCacheDurationKey   = "oauth2ConfigurationCacheTime"
	oauth2CallbackPrefix     = "/internal/oauth2"
	oauth2TokenCookiePrefix  = "z-token-"
	oauth2VerifyCookiePrefix = "z-verifier-"
)

var ErrForbidden = errors.New("forbidden")

type OAuth2RouterOptions struct {
	OAuth2ServerURL              string // Legacy single-tenant settings
	OAuth2TokenURL               string
	OAuth2ClientId               string
	OAuth2ClientSecret           string
	OAuth2WellKnownUrl           string
	OAuth2UserInfoUrl            string
	OAuth2Scopes                 string
	OAuth2CodeChallengeMethod    string
	OAuth2ConfigurationCacheTime *time.Duration
	Logger                       *logger.Logger
	Database                     *database.Database
	OAuth2ConfigCache            *ttlcache.Cache[string, *resolvedOAuth2Config]
	TenantUsageChecker           func(string) []string
}

type OIDCDiscoveryDocument struct {
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	ClaimsSupported                   []string `json:"claims_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	Issuer                            string   `json:"issuer"`
	JwksURI                           string   `json:"jwks_uri"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	UserinfoEndpoint                  string   `json:"userinfo_endpoint"`
}

type OAuth2Tenant struct {
	ID                          string `json:"id"`
	Name                        string `json:"name"`
	OAuth2ServerURL             string `json:"oauth2ServerUrl"`
	OAuth2TokenURL              string `json:"oauth2TokenUrl"`
	OAuth2ClientID              string `json:"oauth2ClientId"`
	OAuth2ClientSecret          string `json:"oauth2ClientSecret"`
	OAuth2WellKnownURL          string `json:"oauth2WellKnownUrl"`
	OAuth2UserInfoURL           string `json:"oauth2UserInfoUrl"`
	OAuth2Scopes                string `json:"oauth2Scopes"`
	OAuth2CodeChallengeMethod   string `json:"oauth2CodeChallengeMethod"`
	OAuth2ConfigurationCacheTTL string `json:"oauth2ConfigurationCacheTime"`
}

type OAuth2TenantSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	IsDefault  bool   `json:"isDefault"`
}

type ClaimRequirement struct {
	Claim  string   `json:"claim"`
	Values []string `json:"values"`
}

type resolvedOAuth2Config struct {
	Config      *gooauth2.Config
	UserInfoURL string
}

type OAuth2Router struct {
	options *OAuth2RouterOptions

	tenants      map[string]*OAuth2Tenant
	tenantsMutex sync.RWMutex
}

// NewOAuth2Router creates a new OAuth2Router object
func NewOAuth2Router(options *OAuth2RouterOptions) *OAuth2Router {
	options.Database.NewTable(oauth2TableName)

	if options.OAuth2ConfigurationCacheTime == nil || options.OAuth2ConfigurationCacheTime.Seconds() == 0 {
		cacheTime := DefaultOAuth2ConfigCacheTime
		options.OAuth2ConfigurationCacheTime = &cacheTime
	}

	options.OAuth2ConfigCache = ttlcache.New[string, *resolvedOAuth2Config](
		ttlcache.WithTTL[string, *resolvedOAuth2Config](DefaultOAuth2ConfigCacheTime),
	)

	ar := &OAuth2Router{
		options: options,
		tenants: map[string]*OAuth2Tenant{},
	}
	if err := ar.loadTenantsFromStore(); err != nil {
		options.Logger.PrintAndLog("OAuth2Router", "Failed to load OAuth2 tenants", err)
	}

	go options.OAuth2ConfigCache.Start()
	return ar
}

func (ar *OAuth2Router) loadTenantsFromStore() error {
	loadedTenants := map[string]*OAuth2Tenant{}
	if err := ar.options.Database.Read(oauth2TableName, oauth2TenantsKey, &loadedTenants); err != nil || len(loadedTenants) == 0 {
		defaultTenant := ar.readLegacyDefaultTenant()
		loadedTenants = map[string]*OAuth2Tenant{
			DefaultTenantID: defaultTenant,
		}
	}

	normalized := map[string]*OAuth2Tenant{}
	for _, tenant := range loadedTenants {
		normalizedTenant := ar.normalizeTenant(tenant)
		normalized[normalizedTenant.ID] = normalizedTenant
	}
	if _, ok := normalized[DefaultTenantID]; !ok {
		normalized[DefaultTenantID] = ar.defaultBlankTenant()
	}

	ar.tenantsMutex.Lock()
	ar.tenants = normalized
	ar.tenantsMutex.Unlock()

	return ar.persistTenants()
}

func (ar *OAuth2Router) defaultBlankTenant() *OAuth2Tenant {
	return &OAuth2Tenant{
		ID:                          DefaultTenantID,
		Name:                        "Default",
		OAuth2CodeChallengeMethod:   "plain",
		OAuth2ConfigurationCacheTTL: DefaultOAuth2ConfigCacheTime.String(),
	}
}

func (ar *OAuth2Router) readLegacyDefaultTenant() *OAuth2Tenant {
	defaultTenant := ar.defaultBlankTenant()

	defaultTenant.OAuth2ServerURL = firstNonEmpty(ar.readLegacyString(legacyServerURLKey), strings.TrimSpace(ar.options.OAuth2ServerURL))
	defaultTenant.OAuth2TokenURL = firstNonEmpty(ar.readLegacyString(legacyTokenURLKey), strings.TrimSpace(ar.options.OAuth2TokenURL))
	defaultTenant.OAuth2ClientID = firstNonEmpty(ar.readLegacyString(legacyClientIDKey), strings.TrimSpace(ar.options.OAuth2ClientId))
	defaultTenant.OAuth2ClientSecret = firstNonEmpty(ar.readLegacyString(legacyClientSecretKey), strings.TrimSpace(ar.options.OAuth2ClientSecret))
	defaultTenant.OAuth2WellKnownURL = firstNonEmpty(ar.readLegacyString(legacyWellKnownKey), strings.TrimSpace(ar.options.OAuth2WellKnownUrl))
	defaultTenant.OAuth2UserInfoURL = firstNonEmpty(ar.readLegacyString(legacyUserInfoURLKey), strings.TrimSpace(ar.options.OAuth2UserInfoUrl))
	defaultTenant.OAuth2Scopes = firstNonEmpty(ar.readLegacyString(legacyScopesKey), strings.TrimSpace(ar.options.OAuth2Scopes))
	defaultTenant.OAuth2CodeChallengeMethod = firstNonEmpty(ar.readLegacyString(legacyCodeChallengeKey), strings.TrimSpace(ar.options.OAuth2CodeChallengeMethod), "plain")

	if cacheTTL := ar.readLegacyDurationString(); cacheTTL != "" {
		defaultTenant.OAuth2ConfigurationCacheTTL = cacheTTL
	}

	return ar.normalizeTenant(defaultTenant)
}

func (ar *OAuth2Router) readLegacyString(key string) string {
	var value string
	_ = ar.options.Database.Read(oauth2TableName, key, &value)
	return strings.TrimSpace(value)
}

func (ar *OAuth2Router) readLegacyDurationString() string {
	var durationValue time.Duration
	if err := ar.options.Database.Read(oauth2TableName, legacyCacheDurationKey, &durationValue); err == nil && durationValue > 0 {
		return durationValue.String()
	}

	if ar.options.OAuth2ConfigurationCacheTime != nil && *ar.options.OAuth2ConfigurationCacheTime > 0 {
		return ar.options.OAuth2ConfigurationCacheTime.String()
	}

	return DefaultOAuth2ConfigCacheTime.String()
}

func (ar *OAuth2Router) persistTenants() error {
	ar.tenantsMutex.RLock()
	tenantsCopy := make(map[string]*OAuth2Tenant, len(ar.tenants))
	for id, tenant := range ar.tenants {
		tenantsCopy[id] = ar.copyTenant(tenant)
	}
	ar.tenantsMutex.RUnlock()

	if err := ar.options.Database.Write(oauth2TableName, oauth2TenantsKey, tenantsCopy); err != nil {
		return err
	}

	defaultTenant, err := ar.GetTenant(DefaultTenantID)
	if err != nil {
		defaultTenant = ar.defaultBlankTenant()
	}
	if err := ar.syncLegacyDefaultTenant(defaultTenant); err != nil {
		return err
	}

	ar.options.OAuth2ConfigCache.DeleteAll()
	return nil
}

func (ar *OAuth2Router) syncLegacyDefaultTenant(tenant *OAuth2Tenant) error {
	if tenant == nil {
		tenant = ar.defaultBlankTenant()
	}

	writeOrDelete := func(key string, value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return ar.options.Database.Delete(oauth2TableName, key)
		}
		return ar.options.Database.Write(oauth2TableName, key, value)
	}

	if err := writeOrDelete(legacyWellKnownKey, tenant.OAuth2WellKnownURL); err != nil {
		return err
	}
	if err := writeOrDelete(legacyServerURLKey, tenant.OAuth2ServerURL); err != nil {
		return err
	}
	if err := writeOrDelete(legacyTokenURLKey, tenant.OAuth2TokenURL); err != nil {
		return err
	}
	if err := writeOrDelete(legacyUserInfoURLKey, tenant.OAuth2UserInfoURL); err != nil {
		return err
	}
	if err := writeOrDelete(legacyClientIDKey, tenant.OAuth2ClientID); err != nil {
		return err
	}
	if err := writeOrDelete(legacyClientSecretKey, tenant.OAuth2ClientSecret); err != nil {
		return err
	}
	if err := writeOrDelete(legacyScopesKey, tenant.OAuth2Scopes); err != nil {
		return err
	}
	if err := writeOrDelete(legacyCodeChallengeKey, tenant.OAuth2CodeChallengeMethod); err != nil {
		return err
	}

	cacheDuration := ar.parseTenantCacheDuration(tenant)
	return ar.options.Database.Write(oauth2TableName, legacyCacheDurationKey, cacheDuration)
}

func (ar *OAuth2Router) normalizeTenant(tenant *OAuth2Tenant) *OAuth2Tenant {
	if tenant == nil {
		tenant = &OAuth2Tenant{}
	}

	normalized := *tenant
	normalized.ID = ar.sanitizeTenantID(normalized.ID)
	if normalized.ID == "" {
		normalized.ID = DefaultTenantID
	}

	normalized.Name = strings.TrimSpace(normalized.Name)
	if normalized.Name == "" {
		if normalized.ID == DefaultTenantID {
			normalized.Name = "Default"
		} else {
			normalized.Name = normalized.ID
		}
	}

	normalized.OAuth2WellKnownURL = strings.TrimSpace(normalized.OAuth2WellKnownURL)
	normalized.OAuth2ServerURL = strings.TrimSpace(normalized.OAuth2ServerURL)
	normalized.OAuth2TokenURL = strings.TrimSpace(normalized.OAuth2TokenURL)
	normalized.OAuth2ClientID = strings.TrimSpace(normalized.OAuth2ClientID)
	normalized.OAuth2ClientSecret = strings.TrimSpace(normalized.OAuth2ClientSecret)
	normalized.OAuth2UserInfoURL = strings.TrimSpace(normalized.OAuth2UserInfoURL)
	normalized.OAuth2Scopes = strings.TrimSpace(normalized.OAuth2Scopes)
	normalized.OAuth2CodeChallengeMethod = normalizeCodeChallengeMethod(normalized.OAuth2CodeChallengeMethod)
	normalized.OAuth2ConfigurationCacheTTL = ar.normalizeCacheDuration(normalized.OAuth2ConfigurationCacheTTL)

	return &normalized
}

func (ar *OAuth2Router) sanitizeTenantID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case unicode.IsLower(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}

	sanitized := strings.Trim(b.String(), "-_")
	if sanitized == "" {
		return ""
	}

	return sanitized
}

func normalizeCodeChallengeMethod(method string) string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "PKCE":
		return "PKCE"
	case "PKCE_S256", "S256":
		return "PKCE_S256"
	default:
		return "plain"
	}
}

func (ar *OAuth2Router) normalizeCacheDuration(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultOAuth2ConfigCacheTime.String()
	}

	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return DefaultOAuth2ConfigCacheTime.String()
	}

	return duration.String()
}

func (ar *OAuth2Router) parseTenantCacheDuration(tenant *OAuth2Tenant) time.Duration {
	if tenant == nil {
		return DefaultOAuth2ConfigCacheTime
	}

	duration, err := time.ParseDuration(strings.TrimSpace(tenant.OAuth2ConfigurationCacheTTL))
	if err != nil || duration <= 0 {
		return DefaultOAuth2ConfigCacheTime
	}
	return duration
}

func (ar *OAuth2Router) copyTenant(tenant *OAuth2Tenant) *OAuth2Tenant {
	if tenant == nil {
		return nil
	}
	copied := *tenant
	return &copied
}

func (ar *OAuth2Router) ListTenants() []OAuth2TenantSummary {
	ar.tenantsMutex.RLock()
	defer ar.tenantsMutex.RUnlock()

	results := make([]OAuth2TenantSummary, 0, len(ar.tenants))
	for _, tenant := range ar.tenants {
		results = append(results, OAuth2TenantSummary{
			ID:         tenant.ID,
			Name:       tenant.Name,
			Configured: tenantConfigured(tenant),
			IsDefault:  tenant.ID == DefaultTenantID,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].IsDefault != results[j].IsDefault {
			return results[i].IsDefault
		}
		return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
	})

	return results
}

func tenantConfigured(tenant *OAuth2Tenant) bool {
	if tenant == nil {
		return false
	}
	return strings.TrimSpace(tenant.OAuth2ClientID) != "" &&
		strings.TrimSpace(tenant.OAuth2ClientSecret) != "" &&
		(strings.TrimSpace(tenant.OAuth2WellKnownURL) != "" ||
			(strings.TrimSpace(tenant.OAuth2ServerURL) != "" &&
				strings.TrimSpace(tenant.OAuth2TokenURL) != "" &&
				strings.TrimSpace(tenant.OAuth2UserInfoURL) != ""))
}

func (ar *OAuth2Router) GetTenant(id string) (*OAuth2Tenant, error) {
	id = ar.sanitizeTenantID(id)
	if id == "" {
		id = DefaultTenantID
	}

	ar.tenantsMutex.RLock()
	defer ar.tenantsMutex.RUnlock()

	tenant, ok := ar.tenants[id]
	if !ok {
		return nil, fmt.Errorf("OAuth2 tenant %q not found", id)
	}
	return ar.copyTenant(tenant), nil
}

func (ar *OAuth2Router) SetTenant(tenant *OAuth2Tenant) (*OAuth2Tenant, error) {
	normalized := ar.normalizeTenant(tenant)

	ar.tenantsMutex.Lock()
	ar.tenants[normalized.ID] = normalized
	ar.tenantsMutex.Unlock()

	if err := ar.persistTenants(); err != nil {
		return nil, err
	}
	return ar.copyTenant(normalized), nil
}

func (ar *OAuth2Router) CreateTenant(name string, id string) (*OAuth2Tenant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("tenant name cannot be empty")
	}

	explicitID := strings.TrimSpace(id)
	baseID := ar.sanitizeTenantID(id)
	if baseID == "" {
		baseID = ar.sanitizeTenantID(name)
	}
	if baseID == "" {
		baseID = "tenant"
	}
	if baseID == DefaultTenantID && strings.ToLower(explicitID) != DefaultTenantID {
		baseID = "tenant"
	}

	ar.tenantsMutex.Lock()
	if explicitID != "" {
		if _, exists := ar.tenants[baseID]; exists {
			ar.tenantsMutex.Unlock()
			return nil, fmt.Errorf("OAuth2 tenant %q already exists", baseID)
		}
	} else {
		baseID = ar.nextAvailableTenantIDLocked(baseID)
	}

	tenant := ar.normalizeTenant(&OAuth2Tenant{
		ID:                          baseID,
		Name:                        name,
		OAuth2CodeChallengeMethod:   "plain",
		OAuth2ConfigurationCacheTTL: DefaultOAuth2ConfigCacheTime.String(),
	})

	ar.tenants[tenant.ID] = tenant
	ar.tenantsMutex.Unlock()
	if err := ar.persistTenants(); err != nil {
		return nil, err
	}
	return ar.copyTenant(tenant), nil
}

func (ar *OAuth2Router) nextAvailableTenantIDLocked(base string) string {
	candidate := base
	index := 2
	for {
		if _, exists := ar.tenants[candidate]; !exists {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, index)
		index++
	}
}

func (ar *OAuth2Router) DeleteTenant(id string) error {
	id = ar.sanitizeTenantID(id)
	if id == "" {
		id = DefaultTenantID
	}

	ar.tenantsMutex.Lock()

	if id == DefaultTenantID {
		name := "Default"
		if existing, ok := ar.tenants[DefaultTenantID]; ok && strings.TrimSpace(existing.Name) != "" {
			name = existing.Name
		}
		ar.tenants[DefaultTenantID] = ar.normalizeTenant(&OAuth2Tenant{
			ID:                          DefaultTenantID,
			Name:                        name,
			OAuth2CodeChallengeMethod:   "plain",
			OAuth2ConfigurationCacheTTL: DefaultOAuth2ConfigCacheTime.String(),
		})
	} else {
		if _, ok := ar.tenants[id]; !ok {
			ar.tenantsMutex.Unlock()
			return fmt.Errorf("OAuth2 tenant %q not found", id)
		}
		delete(ar.tenants, id)
	}

	ar.tenantsMutex.Unlock()
	return ar.persistTenants()
}

// HandleSetOAuth2Settings keeps the legacy single-tenant API wired to the default tenant.
func (ar *OAuth2Router) HandleSetOAuth2Settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenant, err := ar.GetTenant(DefaultTenantID)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		ar.sendTenantResponse(w, tenant)
	case http.MethodPost:
		tenant, err := ar.parseTenantFromRequest(r, DefaultTenantID)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		if _, err := ar.SetTenant(tenant); err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		utils.SendOK(w)
	case http.MethodDelete:
		if err := ar.DeleteTenant(DefaultTenantID); err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		utils.SendOK(w)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (ar *OAuth2Router) HandleTenantListAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		js, _ := json.Marshal(ar.ListTenants())
		utils.SendJSONResponse(w, string(js))
	case http.MethodPost:
		name, err := utils.PostPara(r, "name")
		if err != nil {
			utils.SendErrorResponse(w, "tenant name not found")
			return
		}
		customID, _ := utils.PostPara(r, "id")
		tenant, err := ar.CreateTenant(name, customID)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		ar.sendTenantResponse(w, tenant)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (ar *OAuth2Router) HandleTenantAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenantID := ar.getTenantIDFromRequest(r, DefaultTenantID)
		tenant, err := ar.GetTenant(tenantID)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		ar.sendTenantResponse(w, tenant)
	case http.MethodPost:
		tenantID := ar.getTenantIDFromRequest(r, "")
		if tenantID == "" {
			utils.SendErrorResponse(w, "tenant id not found")
			return
		}
		existingTenant, _ := ar.GetTenant(tenantID)
		tenant, err := ar.parseTenantFromRequest(r, tenantID)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		if existingTenant != nil && tenant.Name == "" {
			tenant.Name = existingTenant.Name
		}
		if _, err := ar.SetTenant(tenant); err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		utils.SendOK(w)
	case http.MethodDelete:
		tenantID := ar.getTenantIDFromRequest(r, DefaultTenantID)
		if tenantID != DefaultTenantID && ar.options.TenantUsageChecker != nil {
			inUseBy := ar.options.TenantUsageChecker(tenantID)
			if len(inUseBy) > 0 {
				utils.SendErrorResponse(w, fmt.Sprintf("OAuth2 tenant %q is still used by host rules: %s", tenantID, strings.Join(inUseBy, ", ")))
				return
			}
		}
		if err := ar.DeleteTenant(tenantID); err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		utils.SendOK(w)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (ar *OAuth2Router) getTenantIDFromRequest(r *http.Request, fallback string) string {
	var tenantID string
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodDelete:
		tenantID, _ = utils.GetPara(r, "id")
		if tenantID == "" {
			tenantID, _ = utils.GetPara(r, "tenantId")
		}
	default:
		tenantID, _ = utils.PostPara(r, "id")
		if tenantID == "" {
			tenantID, _ = utils.PostPara(r, "tenantId")
		}
	}

	tenantID = ar.sanitizeTenantID(tenantID)
	if tenantID == "" {
		tenantID = fallback
	}
	return tenantID
}

func (ar *OAuth2Router) parseTenantFromRequest(r *http.Request, tenantID string) (*OAuth2Tenant, error) {
	var oauth2ServerURL string
	var oauth2TokenURL string
	var oauth2Scopes string
	var oauth2UserInfoURL string

	oauth2ClientID, err := utils.PostPara(r, "oauth2ClientId")
	if err != nil {
		return nil, errors.New("oauth2ClientId not found")
	}

	oauth2ClientSecret, err := utils.PostPara(r, "oauth2ClientSecret")
	if err != nil {
		return nil, errors.New("oauth2ClientSecret not found")
	}

	oauth2CodeChallengeMethod, err := utils.PostPara(r, "oauth2CodeChallengeMethod")
	if err != nil {
		return nil, errors.New("oauth2CodeChallengeMethod not found")
	}

	oauth2ConfigurationCacheTime, err := utils.PostDuration(r, "oauth2ConfigurationCacheTime")
	if err != nil {
		return nil, errors.New("oauth2ConfigurationCacheTime not found")
	}

	oauth2WellKnownURL, err := utils.PostPara(r, "oauth2WellKnownUrl")
	if err != nil {
		oauth2ServerURL, err = utils.PostPara(r, "oauth2ServerUrl")
		if err != nil {
			return nil, errors.New("oauth2ServerUrl not found")
		}

		oauth2TokenURL, err = utils.PostPara(r, "oauth2TokenUrl")
		if err != nil {
			return nil, errors.New("oauth2TokenUrl not found")
		}

		oauth2UserInfoURL, err = ar.postParamAny(r, "oauth2UserInfoUrl", "oauth2UserInfoURL")
		if err != nil {
			return nil, errors.New("oauth2UserInfoUrl not found")
		}

		oauth2Scopes, err = utils.PostPara(r, "oauth2Scopes")
		if err != nil {
			return nil, errors.New("oauth2Scopes not found")
		}
	} else {
		oauth2Scopes, _ = utils.PostPara(r, "oauth2Scopes")
		oauth2ServerURL, _ = utils.PostPara(r, "oauth2ServerUrl")
		oauth2TokenURL, _ = utils.PostPara(r, "oauth2TokenUrl")
		oauth2UserInfoURL, _ = ar.postParamAny(r, "oauth2UserInfoUrl", "oauth2UserInfoURL")
	}

	tenantName, _ := utils.PostPara(r, "name")
	if tenantName == "" {
		tenantName, _ = utils.PostPara(r, "tenantName")
	}

	return ar.normalizeTenant(&OAuth2Tenant{
		ID:                          tenantID,
		Name:                        tenantName,
		OAuth2ServerURL:             oauth2ServerURL,
		OAuth2TokenURL:              oauth2TokenURL,
		OAuth2ClientID:              oauth2ClientID,
		OAuth2ClientSecret:          oauth2ClientSecret,
		OAuth2WellKnownURL:          oauth2WellKnownURL,
		OAuth2UserInfoURL:           oauth2UserInfoURL,
		OAuth2Scopes:                oauth2Scopes,
		OAuth2CodeChallengeMethod:   oauth2CodeChallengeMethod,
		OAuth2ConfigurationCacheTTL: oauth2ConfigurationCacheTime.String(),
	}), nil
}

func (ar *OAuth2Router) sendTenantResponse(w http.ResponseWriter, tenant *OAuth2Tenant) {
	if tenant == nil {
		tenant = ar.defaultBlankTenant()
	}
	js, _ := json.Marshal(map[string]interface{}{
		"id":                           tenant.ID,
		"name":                         tenant.Name,
		"isDefault":                    tenant.ID == DefaultTenantID,
		"oauth2WellKnownUrl":           tenant.OAuth2WellKnownURL,
		"oauth2ServerUrl":              tenant.OAuth2ServerURL,
		"oauth2TokenUrl":               tenant.OAuth2TokenURL,
		"oauth2UserInfoUrl":            tenant.OAuth2UserInfoURL,
		"oauth2Scopes":                 tenant.OAuth2Scopes,
		"oauth2ClientSecret":           tenant.OAuth2ClientSecret,
		"oauth2ClientId":               tenant.OAuth2ClientID,
		"oauth2CodeChallengeMethod":    tenant.OAuth2CodeChallengeMethod,
		"oauth2ConfigurationCacheTime": tenant.OAuth2ConfigurationCacheTTL,
	})
	utils.SendJSONResponse(w, string(js))
}

func (ar *OAuth2Router) fetchOAuth2Configuration(tenant *OAuth2Tenant, config *gooauth2.Config) (*resolvedOAuth2Config, error) {
	req, err := http.NewRequest(http.MethodGet, tenant.OAuth2WellKnownURL, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	oidcDiscoveryDocument := OIDCDiscoveryDocument{}
	if err := json.NewDecoder(resp.Body).Decode(&oidcDiscoveryDocument); err != nil {
		ar.options.Logger.PrintAndLog("OAuth2Router", fmt.Sprintf("Failed to decode ([%d] %s)", resp.StatusCode, resp.Status), err)
		return nil, err
	}

	if len(config.Scopes) == 0 {
		config.Scopes = oidcDiscoveryDocument.ScopesSupported
	}
	if config.Endpoint.AuthURL == "" {
		config.Endpoint.AuthURL = oidcDiscoveryDocument.AuthorizationEndpoint
	}
	if config.Endpoint.TokenURL == "" {
		config.Endpoint.TokenURL = oidcDiscoveryDocument.TokenEndpoint
	}

	userInfoURL := tenant.OAuth2UserInfoURL
	if userInfoURL == "" {
		userInfoURL = oidcDiscoveryDocument.UserinfoEndpoint
	}

	return &resolvedOAuth2Config{
		Config:      config,
		UserInfoURL: userInfoURL,
	}, nil
}

func (ar *OAuth2Router) newOAuth2Conf(tenant *OAuth2Tenant, redirectURL string) (*resolvedOAuth2Config, error) {
	config := &gooauth2.Config{
		ClientID:     tenant.OAuth2ClientID,
		ClientSecret: tenant.OAuth2ClientSecret,
		RedirectURL:  redirectURL,
		Endpoint: gooauth2.Endpoint{
			AuthURL:  tenant.OAuth2ServerURL,
			TokenURL: tenant.OAuth2TokenURL,
		},
	}

	if tenant.OAuth2Scopes != "" {
		config.Scopes = splitAndTrimCSV(tenant.OAuth2Scopes)
	}

	if tenant.OAuth2WellKnownURL != "" && (config.Endpoint.AuthURL == "" || config.Endpoint.TokenURL == "" || tenant.OAuth2UserInfoURL == "") {
		return ar.fetchOAuth2Configuration(tenant, config)
	}

	return &resolvedOAuth2Config{
		Config:      config,
		UserInfoURL: tenant.OAuth2UserInfoURL,
	}, nil
}

func NormalizeClaimRequirements(requirements []*ClaimRequirement) []*ClaimRequirement {
	if len(requirements) == 0 {
		return []*ClaimRequirement{}
	}

	normalized := make([]*ClaimRequirement, 0, len(requirements))
	for _, requirement := range requirements {
		if requirement == nil {
			continue
		}

		claimName := strings.TrimSpace(requirement.Claim)
		if claimName == "" {
			continue
		}

		values := make([]string, 0, len(requirement.Values))
		for _, value := range requirement.Values {
			value = strings.TrimSpace(value)
			if value != "" {
				values = append(values, value)
			}
		}

		normalized = append(normalized, &ClaimRequirement{
			Claim:  claimName,
			Values: values,
		})
	}

	return normalized
}

func resolveClaimPath(source interface{}, path string) (interface{}, bool) {
	current := source
	for _, part := range strings.Split(strings.TrimSpace(path), ".") {
		if strings.TrimSpace(part) == "" {
			return nil, false
		}

		obj, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}

		next, exists := obj[part]
		if !exists {
			return nil, false
		}
		current = next
	}

	return current, true
}

func flattenClaimValues(value interface{}) []string {
	switch typed := value.(type) {
	case nil:
		return []string{}
	case string:
		return []string{typed}
	case bool:
		return []string{fmt.Sprint(typed)}
	case float64:
		return []string{fmt.Sprint(typed)}
	case int:
		return []string{fmt.Sprint(typed)}
	case json.Number:
		return []string{typed.String()}
	case []string:
		return typed
	case []interface{}:
		values := []string{}
		for _, entry := range typed {
			values = append(values, flattenClaimValues(entry)...)
		}
		return values
	default:
		return []string{fmt.Sprint(typed)}
	}
}

func claimsMatchUserInfo(userInfo map[string]interface{}, requirements []*ClaimRequirement) bool {
	normalizedRequirements := NormalizeClaimRequirements(requirements)
	if len(normalizedRequirements) == 0 {
		return true
	}

	for _, requirement := range normalizedRequirements {
		resolvedValue, ok := resolveClaimPath(userInfo, requirement.Claim)
		if !ok {
			return false
		}

		resolvedValues := flattenClaimValues(resolvedValue)
		if len(requirement.Values) == 0 {
			if len(resolvedValues) == 0 {
				return false
			}
			continue
		}

		matched := false
		for _, actualValue := range resolvedValues {
			for _, expectedValue := range requirement.Values {
				if strings.EqualFold(strings.TrimSpace(actualValue), strings.TrimSpace(expectedValue)) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}

		if !matched {
			return false
		}
	}

	return true
}

// HandleOAuth2Auth is the internal handler for OAuth authentication.
func (ar *OAuth2Router) HandleOAuth2Auth(w http.ResponseWriter, r *http.Request, requestedTenantID string, requiredClaims []*ClaimRequirement) error {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	tenantID := ar.resolveRequestTenantID(r, requestedTenantID)
	tenant, err := ar.GetTenant(tenantID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}

	reqURL := scheme + "://" + r.Host + r.RequestURI
	resolvedConfig, err := ar.getResolvedConfig(scheme, r.Host, tenant)
	if err != nil {
		ar.options.Logger.PrintAndLog("OAuth2Router", "Failed to fetch OIDC configuration", err)
		w.WriteHeader(http.StatusInternalServerError)
		return errors.New("failed to fetch OIDC configuration")
	}

	oauthConfig := resolvedConfig.Config
	if oauthConfig == nil || oauthConfig.Endpoint.AuthURL == "" || oauthConfig.Endpoint.TokenURL == "" || resolvedConfig.UserInfoURL == "" {
		ar.options.Logger.PrintAndLog("OAuth2Router", "Invalid OAuth2 configuration", nil)
		w.WriteHeader(http.StatusInternalServerError)
		return errors.New("invalid OAuth2 configuration")
	}

	tokenCookieName := oauth2TokenCookiePrefix + tenant.ID
	verifierCookieName := oauth2VerifyCookiePrefix + tenant.ID

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if r.Method == http.MethodGet && r.URL.Path == oauth2CallbackPrefix && code != "" && state != "" {
		ctx := context.Background()
		var authCodeOptions []gooauth2.AuthCodeOption
		if tenant.OAuth2CodeChallengeMethod == "PKCE" || tenant.OAuth2CodeChallengeMethod == "PKCE_S256" {
			verifierCookie, err := r.Cookie(verifierCookieName)
			if err != nil || verifierCookie.Value == "" {
				ar.options.Logger.PrintAndLog("OAuth2Router", "Read OAuth2 verifier cookie failed", err)
				w.WriteHeader(http.StatusUnauthorized)
				return errors.New("unauthorized")
			}
			authCodeOptions = append(authCodeOptions, gooauth2.VerifierOption(verifierCookie.Value))
		}

		token, err := oauthConfig.Exchange(ctx, code, authCodeOptions...)
		if err != nil {
			ar.options.Logger.PrintAndLog("OAuth2", "Token exchange failed", err)
			w.WriteHeader(http.StatusUnauthorized)
			return errors.New("unauthorized")
		}
		if !token.Valid() {
			ar.options.Logger.PrintAndLog("OAuth2", "Invalid token", err)
			w.WriteHeader(http.StatusUnauthorized)
			return errors.New("unauthorized")
		}

		cookieExpiry := token.Expiry
		if cookieExpiry.IsZero() || cookieExpiry.Before(time.Now()) {
			cookieExpiry = time.Now().Add(time.Hour)
		}

		tokenCookie := http.Cookie{Name: tokenCookieName, Value: token.AccessToken, Path: "/", Expires: cookieExpiry}
		if scheme == "https" {
			tokenCookie.Secure = true
			tokenCookie.SameSite = http.SameSiteLaxMode
		}
		w.Header().Add("Set-Cookie", tokenCookie.String())

		if tenant.OAuth2CodeChallengeMethod == "PKCE" || tenant.OAuth2CodeChallengeMethod == "PKCE_S256" {
			verifierCookie := http.Cookie{Name: verifierCookieName, Value: "", Path: "/", Expires: time.Now().Add(-time.Hour)}
			if scheme == "https" {
				verifierCookie.Secure = true
				verifierCookie.SameSite = http.SameSiteLaxMode
			}
			w.Header().Add("Set-Cookie", verifierCookie.String())
		}

		location := strings.TrimPrefix(state, "/internal/")
		decodedLocation, err := url.PathUnescape(location)
		if err == nil && (strings.HasPrefix(decodedLocation, "http://") || strings.HasPrefix(decodedLocation, "https://")) {
			http.Redirect(w, r, decodedLocation, http.StatusTemporaryRedirect)
		} else {
			http.Redirect(w, r, state, http.StatusTemporaryRedirect)
		}
		return errors.New("authorized")
	}

	unauthorized := false
	tokenCookie, err := r.Cookie(tokenCookieName)
	if err == nil {
		if tokenCookie.Value == "" {
			unauthorized = true
		} else {
			ctx := context.Background()
			client := oauthConfig.Client(ctx, &gooauth2.Token{AccessToken: tokenCookie.Value})
			req, err := client.Get(resolvedConfig.UserInfoURL)
			if err != nil {
				ar.options.Logger.PrintAndLog("OAuth2", "Failed to get user info", err)
				unauthorized = true
			} else {
				defer req.Body.Close()
				if req.StatusCode != http.StatusOK {
					ar.options.Logger.PrintAndLog("OAuth2", "Failed to get user info", fmt.Errorf("status %d", req.StatusCode))
					unauthorized = true
				} else if len(NormalizeClaimRequirements(requiredClaims)) > 0 {
					userInfo := map[string]interface{}{}
					if err := json.NewDecoder(req.Body).Decode(&userInfo); err != nil {
						ar.options.Logger.PrintAndLog("OAuth2", "Failed to decode user info claims", err)
						w.WriteHeader(http.StatusForbidden)
						w.Write([]byte("403 - Forbidden"))
						return ErrForbidden
					}
					if !claimsMatchUserInfo(userInfo, requiredClaims) {
						ar.options.Logger.PrintAndLog("OAuth2", "OAuth2 claim requirements not satisfied", fmt.Errorf("tenant=%s host=%s", tenant.ID, r.Host))
						w.WriteHeader(http.StatusForbidden)
						w.Write([]byte("403 - Forbidden"))
						return ErrForbidden
					}
				}
			}
		}
	} else {
		unauthorized = true
	}

	if unauthorized {
		state := url.QueryEscape(reqURL)
		var redirectURL string
		if tenant.OAuth2CodeChallengeMethod == "PKCE" || tenant.OAuth2CodeChallengeMethod == "PKCE_S256" {
			verifierCookie := http.Cookie{Name: verifierCookieName, Value: gooauth2.GenerateVerifier(), Path: "/", Expires: time.Now().Add(time.Hour)}
			if scheme == "https" {
				verifierCookie.Secure = true
				verifierCookie.SameSite = http.SameSiteLaxMode
			}

			w.Header().Add("Set-Cookie", verifierCookie.String())

			if tenant.OAuth2CodeChallengeMethod == "PKCE" {
				redirectURL = oauthConfig.AuthCodeURL(state, gooauth2.AccessTypeOffline, gooauth2.SetAuthURLParam("code_challenge", verifierCookie.Value))
			} else {
				redirectURL = oauthConfig.AuthCodeURL(state, gooauth2.AccessTypeOffline, gooauth2.S256ChallengeOption(verifierCookie.Value))
			}
		} else {
			redirectURL = oauthConfig.AuthCodeURL(state, gooauth2.AccessTypeOffline)
		}

		http.Redirect(w, r, redirectURL, http.StatusFound)
		return errors.New("unauthorized")
	}

	return nil
}

func (ar *OAuth2Router) resolveRequestTenantID(r *http.Request, requestedTenantID string) string {
	queryTenantID := ar.sanitizeTenantID(r.URL.Query().Get("tenant"))
	if queryTenantID != "" {
		return queryTenantID
	}

	requestedTenantID = ar.sanitizeTenantID(requestedTenantID)
	if requestedTenantID == "" {
		return DefaultTenantID
	}

	return requestedTenantID
}

func (ar *OAuth2Router) getResolvedConfig(scheme string, host string, tenant *OAuth2Tenant) (*resolvedOAuth2Config, error) {
	cacheKey := fmt.Sprintf("%s|%s|%s", scheme, strings.ToLower(host), tenant.ID)
	if item := ar.options.OAuth2ConfigCache.Get(cacheKey); item != nil {
		return item.Value(), nil
	}

	redirectURL := ar.buildRedirectURL(scheme, host, tenant.ID)
	resolvedConfig, err := ar.newOAuth2Conf(tenant, redirectURL)
	if err != nil {
		return nil, err
	}

	ar.options.OAuth2ConfigCache.Set(cacheKey, resolvedConfig, ar.parseTenantCacheDuration(tenant))
	return resolvedConfig, nil
}

func (ar *OAuth2Router) buildRedirectURL(scheme string, host string, tenantID string) string {
	callbackURL := &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   oauth2CallbackPrefix,
	}
	query := callbackURL.Query()
	query.Set("tenant", tenantID)
	callbackURL.RawQuery = query.Encode()
	return callbackURL.String()
}

func splitAndTrimCSV(raw string) []string {
	if raw == "" {
		return []string{}
	}

	parts := strings.Split(raw, ",")
	results := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			results = append(results, part)
		}
	}
	return results
}

func (ar *OAuth2Router) postParamAny(r *http.Request, keys ...string) (string, error) {
	for _, key := range keys {
		value, err := utils.PostPara(r, key)
		if err == nil {
			return value, nil
		}
	}

	return "", errors.New("parameter not found")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
