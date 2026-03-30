package access

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
)

// Check both blacklist and whitelist for access for both geoIP and ip / CIDR ranges
func (s *AccessRule) AllowIpAccess(ipaddr string) bool {
	if s.IsBlacklisted(ipaddr) {
		return false
	}

	return s.IsWhitelisted(ipaddr)
}

// Check both blacklist and whitelist for access using net.Conn
func (s *AccessRule) AllowConnectionAccess(conn net.Conn) bool {
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		return s.AllowIpAccess(addr.IP.String())
	}
	return true
}

// Toggle blacklist
func (s *AccessRule) ToggleBlacklist(enabled bool) {
	s.BlacklistEnabled = enabled
	s.SaveChanges()
}

// Toggel whitelist
func (s *AccessRule) ToggleWhitelist(enabled bool) {
	s.WhitelistEnabled = enabled
	s.SaveChanges()
}

// Toggle whitelist loopback
func (s *AccessRule) ToggleAllowLoopback(enabled bool) {
	s.WhitelistAllowLocalAndLoopback = enabled
	s.SaveChanges()
}

// Toggle trust proxy headers only
func (s *AccessRule) ToggleTrustProxy(enabled bool) {
	s.TrustProxyHeadersOnly = enabled
	s.SaveChanges()
}

/*
Check if a IP address is blacklisted, in either country or IP blacklist
IsBlacklisted default return is false (allow access)
*/
func (s *AccessRule) IsBlacklisted(ipAddr string) bool {
	if !s.BlacklistEnabled {
		//Blacklist not enabled. Always return false
		return false
	}

	if ipAddr == "" {
		//Unable to get the target IP address
		return false
	}

	countryCode, err := s.parent.Options.GeoDB.ResolveCountryCodeFromIP(ipAddr)
	if err != nil {
		return false
	}

	if s.IsCountryCodeBlacklisted(countryCode.CountryIsoCode) {
		return true
	}

	if s.IsIPBlacklisted(ipAddr) {
		return true
	}

	return false
}

/*
IsWhitelisted check if a given IP address is in the current
server's white list.

Note that the Whitelist default result is true even
when encountered error
*/
func (s *AccessRule) IsWhitelisted(ipAddr string) bool {
	if !s.WhitelistEnabled {
		//Whitelist not enabled. Always return true (allow access)
		return true
	}

	if ipAddr == "" {
		//Unable to get the target IP address, assume ok
		return true
	}

	countryCode, err := s.parent.Options.GeoDB.ResolveCountryCodeFromIP(ipAddr)
	if err != nil {
		return true
	}

	if s.IsCountryCodeWhitelisted(countryCode.CountryIsoCode) {
		return true
	}

	if s.IsIPWhitelisted(ipAddr) {
		return true
	}

	return false
}

/* Utilities function */

// Update the current access rule to json file
func (s *AccessRule) SaveChanges() error {
	if s.parent == nil {
		return errors.New("save failed: access rule detached from controller")
	}
	saveTarget := filepath.Join(s.parent.Options.ConfigFolder, s.ID+".json")
	js, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return err
	}

	err = os.WriteFile(saveTarget, js, 0775)
	return err
}

// Delete this access rule, this will only delete the config file.
// for runtime delete, use DeleteAccessRuleByID from parent Controller
func (s *AccessRule) DeleteConfigFile() error {
	saveTarget := filepath.Join(s.parent.Options.ConfigFolder, s.ID+".json")
	return os.Remove(saveTarget)
}

// Delete the access rule by given ID
func (c *Controller) DeleteAccessRuleByID(accessRuleID string) error {
	targetAccessRule, err := c.GetAccessRuleByID(accessRuleID)
	if err != nil {
		return err
	}

	//Delete config file associated with this access rule
	err = targetAccessRule.DeleteConfigFile()
	if err != nil {
		return err
	}

	//Delete the access rule in runtime
	c.ProxyAccessRule.Delete(accessRuleID)
	return nil
}

// Create a deep copy object of the access rule list
func deepCopy(valueList map[string]string) map[string]string {
	result := map[string]string{}
	js, _ := json.Marshal(valueList)
	json.Unmarshal(js, &result)
	return result
}
