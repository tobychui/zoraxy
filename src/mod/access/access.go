package access

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	Access.go

	This module is the new version of access control system
	where now the blacklist / whitelist are seperated from
	geodb module
*/

// Create a new access controller to handle blacklist / whitelist
func NewAccessController(options *Options) (*Controller, error) {
	sysdb := options.Database
	if sysdb == nil {
		return nil, errors.New("missing database access")
	}

	//Create the config folder if not exists
	confFolder := options.ConfigFolder
	if !utils.FileExists(confFolder) {
		err := os.MkdirAll(confFolder, 0775)
		if err != nil {
			return nil, err
		}
	}

	// Create the global access rule if not exists
	var defaultAccessRule = AccessRule{
		ID:                   "default",
		Name:                 "Default",
		Desc:                 "Default access rule for all HTTP proxy hosts",
		BlacklistEnabled:     false,
		WhitelistEnabled:     false,
		WhiteListCountryCode: &map[string]string{},
		WhiteListIP:          &map[string]string{},
		BlackListContryCode:  &map[string]string{},
		BlackListIP:          &map[string]string{},
	}
	defaultRuleSettingFile := filepath.Join(confFolder, "default.json")
	if utils.FileExists(defaultRuleSettingFile) {
		//Load from file
		defaultRuleBytes, err := os.ReadFile(defaultRuleSettingFile)
		if err == nil {
			err = json.Unmarshal(defaultRuleBytes, &defaultAccessRule)
			if err != nil {
				options.Logger.PrintAndLog("Access", "Unable to parse default routing rule config file. Using default", err)
			}
		}
	} else {
		//Create one
		js, _ := json.MarshalIndent(defaultAccessRule, "", " ")
		os.WriteFile(defaultRuleSettingFile, js, 0775)

	}

	//Generate a controller object
	thisController := Controller{
		DefaultAccessRule: &defaultAccessRule,
		ProxyAccessRule:   &sync.Map{},
		Options:           options,
	}

	//Assign default access rule parent
	thisController.DefaultAccessRule.parent = &thisController

	//Load all acccess rules from file
	configFiles, err := filepath.Glob(options.ConfigFolder + "/*.json")
	if err != nil {
		return nil, err
	}
	ProxyAccessRules := sync.Map{}
	for _, configFile := range configFiles {
		if filepath.Base(configFile) == "default.json" {
			//Skip this, as this was already loaded as default
			continue
		}

		configContent, err := os.ReadFile(configFile)
		if err != nil {
			options.Logger.PrintAndLog("Access", "Unable to load config "+filepath.Base(configFile), err)
			continue
		}

		//Parse the config file into AccessRule
		thisAccessRule := AccessRule{}
		err = json.Unmarshal(configContent, &thisAccessRule)
		if err != nil {
			options.Logger.PrintAndLog("access", "Unable to parse config "+filepath.Base(configFile), err)
			continue
		}
		thisAccessRule.parent = &thisController
		ProxyAccessRules.Store(thisAccessRule.ID, &thisAccessRule)
	}
	thisController.ProxyAccessRule = &ProxyAccessRules

	//Start the public ip ticker
	if options.PublicIpCheckInterval <= 0 {
		options.PublicIpCheckInterval = 12 * 60 * 60 //12 hours
	}
	thisController.ServerPublicIP = "127.0.0.1"
	go func() {
		err = thisController.UpdatePublicIP()
		if err != nil {
			options.Logger.PrintAndLog("access", "Unable to update public IP address", err)
		}

		thisController.StartPublicIPUpdater()
	}()
	return &thisController, nil
}

// Get the global access rule
func (c *Controller) GetGlobalAccessRule() (*AccessRule, error) {
	if c.DefaultAccessRule == nil {
		return nil, errors.New("global access rule is not set")
	}
	return c.DefaultAccessRule, nil
}

// Load access rules to runtime, require rule ID
func (c *Controller) GetAccessRuleByID(accessRuleID string) (*AccessRule, error) {
	if accessRuleID == "default" || accessRuleID == "" {

		return c.DefaultAccessRule, nil
	}
	//Load from sync.Map, should be O(1)
	targetRule, ok := c.ProxyAccessRule.Load(accessRuleID)

	if !ok {
		return nil, errors.New("target access rule not exists")
	}

	ar, ok := targetRule.(*AccessRule)
	if !ok {
		return nil, errors.New("assertion of access rule failed, version too old?")
	}
	return ar, nil
}

// Return all the access rules currently in runtime, including default
func (c *Controller) ListAllAccessRules() []*AccessRule {
	results := []*AccessRule{c.DefaultAccessRule}
	c.ProxyAccessRule.Range(func(key, value interface{}) bool {
		results = append(results, value.(*AccessRule))
		return true
	})

	return results
}

// Check if an access rule exists given the rule id
func (c *Controller) AccessRuleExists(ruleID string) bool {
	r, _ := c.GetAccessRuleByID(ruleID)
	return r != nil
}

// Add a new access rule to runtime and save it to file
func (c *Controller) AddNewAccessRule(newRule *AccessRule) error {
	r, _ := c.GetAccessRuleByID(newRule.ID)
	if r != nil {
		//An access rule with identical ID exists
		return errors.New("access rule already exists")
	}

	//Check if the blacklist and whitelist are populated with empty map
	if newRule.BlackListContryCode == nil {
		newRule.BlackListContryCode = &map[string]string{}
	}
	if newRule.BlackListIP == nil {
		newRule.BlackListIP = &map[string]string{}
	}
	if newRule.WhiteListCountryCode == nil {
		newRule.WhiteListCountryCode = &map[string]string{}
	}
	if newRule.WhiteListIP == nil {
		newRule.WhiteListIP = &map[string]string{}
	}

	//Add access rule to runtime
	newRule.parent = c
	c.ProxyAccessRule.Store(newRule.ID, newRule)

	//Save rule to file
	newRule.SaveChanges()

	return nil
}

// Update the access rule meta info.
func (c *Controller) UpdateAccessRule(ruleID string, name string, desc string) error {
	targetAccessRule, err := c.GetAccessRuleByID(ruleID)
	if err != nil {
		return err
	}

	///Update the name and desc
	targetAccessRule.Name = name
	targetAccessRule.Desc = desc

	//Overwrite the rule currently in sync map
	if ruleID == "default" {
		c.DefaultAccessRule = targetAccessRule
	} else {
		c.ProxyAccessRule.Store(ruleID, targetAccessRule)
	}
	return targetAccessRule.SaveChanges()
}

// Remove the access rule by its id
func (c *Controller) RemoveAccessRuleByID(ruleID string) error {
	if !c.AccessRuleExists(ruleID) {
		return errors.New("access rule not exists")
	}

	//Default cannot be removed
	if ruleID == "default" {
		return errors.New("default access rule cannot be removed")
	}

	//Remove it
	return c.DeleteAccessRuleByID(ruleID)
}

func (c *Controller) Close() {
	c.StopPublicIPUpdater()
}
