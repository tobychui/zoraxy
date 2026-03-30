package events

import (
	"encoding/json"
	"fmt"
)

// EventName represents the type of event
type EventName string

// EventPayload interface for all event payloads
type EventPayload interface {
	// GetName returns the event type
	GetName() EventName

	// Returns the "source" of the event, that is, the component or plugin that emitted the event
	GetEventSource() string
}

// Event represents a system event
type Event struct {
	Name      EventName    `json:"name"`
	Timestamp int64        `json:"timestamp"` // Unix timestamp
	UUID      string       `json:"uuid"`      // UUID for the event
	Data      EventPayload `json:"data"`
}

const (
	// EventBlacklistedIPBlocked is emitted when a blacklisted IP is blocked
	EventBlacklistedIPBlocked EventName = "blacklistedIpBlocked"
	// EventBlacklistToggled is emitted when the blacklist is toggled for an access rule
	EventBlacklistToggled EventName = "blacklistToggled"
	// EventAccessRuleCreated is emitted when a new access ruleset is created
	EventAccessRuleCreated EventName = "accessRuleCreated"
	// A custom event emitted by a plugin, with the intention of being broadcast
	// to the designated recipient(s)
	EventCustom EventName = "customEvent"
	// A dummy event to satisfy the requirement of having at least one event
	EventDummy EventName = "dummy"

	// Add more event types as needed
)

var validEventNames = map[EventName]bool{
	EventBlacklistedIPBlocked: true,
	EventBlacklistToggled:     true,
	EventAccessRuleCreated:    true,
	EventCustom:               true,
	EventDummy:                true,
	// Add more event types as needed
	// NOTE: Keep up-to-date with event names specified above
}

// Check if the event name is valid
func (name EventName) IsValid() bool {
	return validEventNames[name]
}

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

func (e *BlacklistedIPBlockedEvent) GetEventSource() string {
	return "proxy-access"
}

// BlacklistToggledEvent represents an event when the blacklist is disabled for an access rule
type BlacklistToggledEvent struct {
	RuleID  string `json:"rule_id"`
	Enabled bool   `json:"enabled"` // Whether the blacklist is enabled or disabled
}

func (e *BlacklistToggledEvent) GetName() EventName {
	return EventBlacklistToggled
}

func (e *BlacklistToggledEvent) GetEventSource() string {
	return "accesslist-api"
}

// AccessRuleCreatedEvent represents an event when a new access ruleset is created
type AccessRuleCreatedEvent struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Desc             string `json:"desc"`
	BlacklistEnabled bool   `json:"blacklist_enabled"`
	WhitelistEnabled bool   `json:"whitelist_enabled"`
}

func (e *AccessRuleCreatedEvent) GetName() EventName {
	return EventAccessRuleCreated
}

func (e *AccessRuleCreatedEvent) GetEventSource() string {
	return "accesslist-api"
}

type CustomEvent struct {
	SourcePlugin string         `json:"source_plugin"`
	Recipients   []string       `json:"recipients"`
	Payload      map[string]any `json:"payload"`
}

func (e *CustomEvent) GetName() EventName {
	return EventCustom
}

func (e *CustomEvent) GetEventSource() string {
	return e.SourcePlugin
}

// ParseEvent parses a JSON byte slice into an Event struct
func ParseEvent(jsonData []byte, event *Event) error {
	// First, determine the event type, and parse shared fields, from the JSON data
	var temp struct {
		Name      EventName `json:"name"`
		Timestamp int64     `json:"timestamp"`
		UUID      string    `json:"uuid"`
	}
	if err := json.Unmarshal(jsonData, &temp); err != nil {
		return err
	}

	// Set the event name and timestamp
	event.Name = temp.Name
	event.Timestamp = temp.Timestamp
	event.UUID = temp.UUID

	// Now, based on the event type, unmarshal the specific payload
	switch temp.Name {
	case EventBlacklistedIPBlocked:
		type tempData struct {
			Data BlacklistedIPBlockedEvent `json:"data"`
		}
		var payload tempData
		if err := json.Unmarshal(jsonData, &payload); err != nil {
			return err
		}
		event.Data = &payload.Data
	case EventBlacklistToggled:
		type tempData struct {
			Data BlacklistToggledEvent `json:"data"`
		}
		var payload tempData
		if err := json.Unmarshal(jsonData, &payload); err != nil {
			return err
		}
		event.Data = &payload.Data
	case EventAccessRuleCreated:
		type tempData struct {
			Data AccessRuleCreatedEvent `json:"data"`
		}
		var payload tempData
		if err := json.Unmarshal(jsonData, &payload); err != nil {
			return err
		}
		event.Data = &payload.Data
	case EventCustom:
		type tempData struct {
			Data CustomEvent `json:"data"`
		}
		var payload tempData
		if err := json.Unmarshal(jsonData, &payload); err != nil {
			return err
		}
		event.Data = &payload.Data
	default:
		return fmt.Errorf("unknown event: %s, %v", temp.Name, jsonData)
	}
	return nil
}
