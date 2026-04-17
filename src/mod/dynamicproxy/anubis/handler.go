package anubis

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	libanubis "github.com/TecharoHQ/anubis/lib"
	"github.com/TecharoHQ/anubis/lib/policy"
)

type handlerInfo struct {
	handler http.Handler
	config  string
}

var anubisHandlers map[any]handlerInfo = make(map[any]handlerInfo)

func randomHex(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func keyFromHex(value string) (ed25519.PrivateKey, error) {
	keyBytes, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("supplied key is not hex-encoded: %w", err)
	}

	if len(keyBytes) != ed25519.SeedSize {
		return nil, fmt.Errorf("supplied key is not %d bytes long, got %d bytes", ed25519.SeedSize, len(keyBytes))
	}

	return ed25519.NewKeyFromSeed(keyBytes), nil
}

func GetHandler(cfg *AnubisConfig, key any, publicUrl string, final http.Handler) (http.Handler, bool) {
	if cfg == nil {
		return final, false
	}

	ser_bytes, err := json.Marshal(cfg)

	if err != nil {
		log.Fatalf("error encoding config to json: %s", err)
	}

	ser := string(ser_bytes)

	val, ok := anubisHandlers[key]

	if ok && val.config == ser {
		return val.handler, false
	}

	var pol *policy.ParsedConfig

	ctx, exit := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer exit()

	if cfg.Policy == "" {
		pol, err = libanubis.LoadPoliciesOrDefault(ctx, "", cfg.Difficulty, "INFO")
	} else {
		pol, err = policy.ParseConfig(ctx, strings.NewReader(cfg.Policy), "(user)/policy.yaml", cfg.Difficulty, "INFO")
	}

	if err != nil {
		log.Fatalf("can't parse policy file: %v", err)
	}

	pol.Logger.Info("re-creating anubis instance")

	ed25519KeyHex := cfg.Ed25519KeyHex
	needsSave := false

	if ed25519KeyHex == "" {
		pol.Logger.Warn("generating new ed25519 key")
		ed25519KeyHex, err = randomHex(32)

		if err != nil {
			log.Fatalf("failed to generate ed25519 key: %v", err)
		}

		cfg.Ed25519KeyHex = ed25519KeyHex
		needsSave = true
	}

	ed25519Priv, err := keyFromHex(ed25519KeyHex)

	if err != nil {
		log.Fatalf("failed to parse and validate ED25519_PRIVATE_KEY_HEX: %v", err)
	}

	s, err := libanubis.New(libanubis.Options{
		BasePrefix:               "",
		StripBasePrefix:          false,
		Next:                     final,
		Policy:                   pol,
		TargetHost:               "",
		TargetSNI:                "",
		TargetInsecureSkipVerify: false,
		ServeRobotsTXT:           cfg.ServeRobotsTXT,
		ED25519PrivateKey:        ed25519Priv,
		HS512Secret:              []byte(""),
		CookieDomain:             cfg.CookieDomain,
		CookieDynamicDomain:      cfg.CookieDynamicDomain,
		CookieExpiration:         cfg.CookieExpiration, // anubis.CookieDefaultExpirationTime
		CookiePartitioned:        cfg.CookiePartitioned,
		RedirectDomains:          []string{},
		Target:                   "",
		WebmasterEmail:           cfg.WebmasterEmail,
		OpenGraph:                pol.OpenGraph,
		CookieSecure:             cfg.CookieSecure,
		CookieSameSite:           http.SameSiteNoneMode,
		PublicUrl:                publicUrl,
		JWTRestrictionHeader:     "X-Real-IP",
		Logger:                   pol.Logger.With("subsystem", "anubis"),
		DifficultyInJWT:          false,
	})

	var a http.Handler

	a = s
	a = libanubis.AddCustomRealIPHeader("", a)
	a = libanubis.AddRemoteXRealIP(false, "tcp", a)
	a = libanubis.AddXForwardedForToXRealIP(a)
	a = libanubis.AddXForwardedForUpdate(true, a)
	a = libanubis.AddJA4H(a)

	anubisHandlers[key] = handlerInfo{
		handler: a,
		config:  ser,
	}

	return a, needsSave
}
