package plugins

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

// Test (de)serialization of events
func TestEventDeSerialization(t *testing.T) {
	type SerializationTest struct {
		name         string
		event        zoraxy_plugin.Event
		expectedJson string
	}

	timestamp := time.Now().Unix()

	tests := []SerializationTest{
		{
			name: "BlacklistedIPBlocked",
			event: zoraxy_plugin.Event{
				Name:      zoraxy_plugin.EventBlacklistedIPBlocked,
				Timestamp: timestamp,
				Data: &zoraxy_plugin.BlacklistedIPBlockedEvent{
					IP:           "192.168.1.1",
					Comment:      "Test comment",
					RequestedURL: "http://example.com",
					Hostname:     "example.com",
					UserAgent:    "TestUserAgent",
					Method:       "GET",
				},
			},
			expectedJson: `{"name":"blacklistedIpBlocked","timestamp":` + fmt.Sprintf("%d", timestamp) + `,"data":{"ip":"192.168.1.1","comment":"Test comment","requested_url":"http://example.com","hostname":"example.com","user_agent":"TestUserAgent","method":"GET"}}`,
		},
		{
			name: "BlacklistToggled",
			event: zoraxy_plugin.Event{
				Name:      zoraxy_plugin.EventBlacklistToggled,
				Timestamp: timestamp,
				Data: &zoraxy_plugin.BlacklistToggledEvent{
					RuleID:  "rule123",
					Enabled: true,
				},
			},
			expectedJson: `{"name":"blacklistToggled","timestamp":` + fmt.Sprintf("%d", timestamp) + `,"data":{"rule_id":"rule123","enabled":true}}`,
		},
		{
			name: "AccessRuleCreated",
			event: zoraxy_plugin.Event{
				Name:      zoraxy_plugin.EventAccessRuleCreated,
				Timestamp: timestamp,
				Data: &zoraxy_plugin.AccessRuleCreatedEvent{
					ID:               "rule456",
					Name:             "New Access Rule",
					Desc:             "A dummy access rule",
					BlacklistEnabled: true,
					WhitelistEnabled: false,
				},
			},
			expectedJson: `{"name":"accessRuleCreated","timestamp":` + fmt.Sprintf("%d", timestamp) + `,"data":{"id":"rule456","name":"New Access Rule","desc":"A dummy access rule","blacklist_enabled":true,"whitelist_enabled":false}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Serialize the event
			jsonData, err := json.Marshal(test.event)
			if err != nil {
				t.Fatalf("Failed to serialize event: %v", err)
			}

			// Compare the serialized JSON with the expected JSON
			if string(jsonData) != test.expectedJson {
				t.Fatalf("Unexpected JSON output.\nGot:  %s\nWant: %s", jsonData, test.expectedJson)
			}

			// Deserialize the JSON back into an event
			var deserializedEvent zoraxy_plugin.Event
			if err := zoraxy_plugin.ParseEvent(jsonData, &deserializedEvent); err != nil {
				t.Fatalf("Failed to parse event: %v", err)
			}
		})
	}
}
