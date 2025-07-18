package zoraxy_plugin

import (
	"time"
)

// EventName represents the type of event
type EventName string

// EventPayload interface for all event payloads
type EventPayload interface {
	// GetName returns the event type
	GetName() EventName
}

// Event represents a system event
type Event struct {
	Name      EventName    `json:"type"`
	Timestamp time.Time    `json:"timestamp"`
	Data      EventPayload `json:"data"`
}

const (
	// EventBlacklistedIPBlocked is emitted when a blacklisted IP is blocked
	EventBlacklistedIPBlocked EventName = "blacklistedIpBlocked"
	// EventBlacklistToggled is emitted when the blacklist is toggled for an access rule
	EventBlacklistToggled EventName = "blacklistToggled"
	// EventAccessRuleCreated is emitted when a new access ruleset is created
	EventAccessRuleCreated EventName = "accessRuleCreated"

	// Add more event types as needed
)

// BlacklistedIPBlockedEvent represents an event when a blacklisted IP is blocked
type BlacklistedIPBlockedEvent struct {
	IP           string `json:"ip"`
	Comment      string `json:"comment"`
	RequestedURL string `json:"requested_url"`
	Hostname     string `json:"hostname"`
	UserAgent    string `json:"user_agent"`
	Method       string `json:"method"`
}

func (e *BlacklistedIPBlockedEvent) GetName() EventName {
	return EventBlacklistedIPBlocked
}

// BlacklistToggledEvent represents an event when the blacklist is disabled for an access rule
type BlacklistToggledEvent struct {
	RuleID  string `json:"rule_id"`
	Enabled bool   `json:"enabled"` // Whether the blacklist is enabled or disabled
}

func (e *BlacklistToggledEvent) GetName() EventName {
	return EventBlacklistToggled
}

// AccessRuleCreatedEvent represents an event when a new access ruleset is created
type AccessRuleCreatedEvent struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Desc             string `json:"description"`
	BlacklistEnabled bool   `json:"blacklist_enabled"`
	WhitelistEnabled bool   `json:"whitelist_enabled"`
}

func (e *AccessRuleCreatedEvent) GetName() EventName {
	return EventAccessRuleCreated
}
