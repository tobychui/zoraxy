package redirection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

func CloneRedirectRule(rule *RedirectRules) (*RedirectRules, error) {
	if rule == nil {
		return nil, nil
	}

	js, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}

	cloned := &RedirectRules{}
	if err := json.Unmarshal(js, cloned); err != nil {
		return nil, err
	}

	return cloned, nil
}

func (t *RuleTable) ReplaceAllRules(rules []*RedirectRules) error {
	if err := os.MkdirAll(t.configPath, 0775); err != nil {
		return err
	}

	configFiles, err := filepath.Glob(filepath.Join(t.configPath, "*.json"))
	if err != nil {
		return err
	}
	for _, configFile := range configFiles {
		if err := os.Remove(configFile); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	t.rules = sync.Map{}
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].RedirectURL < rules[j].RedirectURL
	})

	for _, rule := range rules {
		if rule == nil {
			continue
		}
		if err := t.AddRedirectRule(rule.RedirectURL, rule.TargetURL, rule.ForwardChildpath, rule.StatusCode, rule.RequireExactMatch, rule.DeviceType, rule.AssignedNodeID); err != nil {
			return err
		}
	}

	return nil
}
