package access

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// GetDefaultAccessRule returns the built-in default access rule.
func GetDefaultAccessRule() *AccessRule {
	return &AccessRule{
		ID:                    "default",
		Name:                  "Default",
		Desc:                  "Default access rule for all HTTP proxy hosts",
		BlacklistEnabled:      false,
		WhitelistEnabled:      false,
		TrustProxyHeadersOnly: false,
		WhiteListCountryCode:  &map[string]string{},
		WhiteListIP:           &map[string]string{},
		BlackListContryCode:   &map[string]string{},
		BlackListIP:           &map[string]string{},
	}
}

// CloneAccessRule creates a detached deep copy of an access rule.
func CloneAccessRule(rule *AccessRule) (*AccessRule, error) {
	if rule == nil {
		return nil, errors.New("access rule cannot be nil")
	}

	js, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}

	cloned := &AccessRule{}
	if err := json.Unmarshal(js, cloned); err != nil {
		return nil, err
	}

	if cloned.WhiteListCountryCode == nil {
		cloned.WhiteListCountryCode = &map[string]string{}
	}
	if cloned.WhiteListIP == nil {
		cloned.WhiteListIP = &map[string]string{}
	}
	if cloned.BlackListContryCode == nil {
		cloned.BlackListContryCode = &map[string]string{}
	}
	if cloned.BlackListIP == nil {
		cloned.BlackListIP = &map[string]string{}
	}

	return cloned, nil
}

// ReplaceAccessRulesOnDisk replaces on-disk access rules and trusted proxy configuration.
func ReplaceAccessRulesOnDisk(configFolder string, trustedProxiesFile string, rules []*AccessRule, trustedProxies []TrustedProxy) error {
	if err := os.MkdirAll(configFolder, 0775); err != nil {
		return err
	}

	configFiles, err := filepath.Glob(filepath.Join(configFolder, "*.json"))
	if err != nil {
		return err
	}

	for _, configFile := range configFiles {
		if err := os.Remove(configFile); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	defaultRuleExists := false
	for _, incomingRule := range rules {
		if incomingRule == nil {
			continue
		}

		clonedRule, err := CloneAccessRule(incomingRule)
		if err != nil {
			return err
		}
		if clonedRule.ID == "default" {
			defaultRuleExists = true
		}

		content, err := json.MarshalIndent(clonedRule, "", " ")
		if err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(configFolder, clonedRule.ID+".json"), content, 0775); err != nil {
			return err
		}
	}

	if !defaultRuleExists {
		defaultRule := GetDefaultAccessRule()
		content, err := json.MarshalIndent(defaultRule, "", " ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(configFolder, defaultRule.ID+".json"), content, 0775); err != nil {
			return err
		}
	}

	trustedProxyContent, err := json.MarshalIndent(trustedProxies, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(trustedProxiesFile, trustedProxyContent, 0775)
}

// ReplaceAllAccessRules replaces all runtime and on-disk access rules and trusted proxies.
func (c *Controller) ReplaceAllAccessRules(rules []*AccessRule, trustedProxies []TrustedProxy) error {
	if c == nil {
		return errors.New("access controller is not initialized")
	}

	if err := ReplaceAccessRulesOnDisk(c.Options.ConfigFolder, c.Options.TrustedProxiesFile, rules, trustedProxies); err != nil {
		return err
	}

	newRules := sync.Map{}
	var defaultRule *AccessRule

	for _, incomingRule := range rules {
		if incomingRule == nil {
			continue
		}

		clonedRule, err := CloneAccessRule(incomingRule)
		if err != nil {
			return err
		}
		clonedRule.parent = c

		if clonedRule.ID == "default" {
			defaultRule = clonedRule
			continue
		}

		newRules.Store(clonedRule.ID, clonedRule)
		if err := clonedRule.SaveChanges(); err != nil {
			return err
		}
	}

	if defaultRule == nil {
		defaultRule = GetDefaultAccessRule()
	}
	defaultRule.parent = c
	if err := defaultRule.SaveChanges(); err != nil {
		return err
	}

	c.DefaultAccessRule = defaultRule
	c.ProxyAccessRule = &newRules

	c.TrustedProxies = sync.Map{}
	for _, trustedProxy := range trustedProxies {
		c.TrustedProxies.Store(trustedProxy.IP, trustedProxy)
	}

	return c.SaveTrustedProxies()
}
