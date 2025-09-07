package eventsystem

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin/events"
)

// Test (de)serialization of events
func TestEventDeSerialization(t *testing.T) {
	type SerializationTest struct {
		name         string
		event        events.Event
		expectedJson string
	}

	timestamp := time.Now().Unix()
	uuid := uuid.New().String()

	tests := []SerializationTest{
		{
			name: "BlacklistedIPBlocked",
			event: events.Event{
				Name:      events.EventBlacklistedIPBlocked,
				Timestamp: timestamp,
				UUID:      uuid,
				Data: &events.BlacklistedIPBlockedEvent{
					IP:           "192.168.1.1",
					Comment:      "Test comment",
					RequestedURL: "http://example.com",
					Hostname:     "example.com",
					UserAgent:    "TestUserAgent",
					Method:       "GET",
				},
			},
			expectedJson: `{"name":"blacklistedIpBlocked","timestamp":` + fmt.Sprintf("%d", timestamp) + `,"uuid":"` + uuid + `","data":{"ip":"192.168.1.1","comment":"Test comment","requested_url":"http://example.com","hostname":"example.com","user_agent":"TestUserAgent","method":"GET"}}`,
		},
		{
			name: "BlacklistToggled",
			event: events.Event{
				Name:      events.EventBlacklistToggled,
				Timestamp: timestamp,
				UUID:      uuid,
				Data: &events.BlacklistToggledEvent{
					RuleID:  "rule123",
					Enabled: true,
				},
			},
			expectedJson: `{"name":"blacklistToggled","timestamp":` + fmt.Sprintf("%d", timestamp) + `,"uuid":"` + uuid + `","data":{"rule_id":"rule123","enabled":true}}`,
		},
		{
			name: "AccessRuleCreated",
			event: events.Event{
				Name:      events.EventAccessRuleCreated,
				Timestamp: timestamp,
				UUID:      uuid,
				Data: &events.AccessRuleCreatedEvent{
					ID:               "rule456",
					Name:             "New Access Rule",
					Desc:             "A dummy access rule",
					BlacklistEnabled: true,
					WhitelistEnabled: false,
				},
			},
			expectedJson: `{"name":"accessRuleCreated","timestamp":` + fmt.Sprintf("%d", timestamp) + `,"uuid":"` + uuid + `","data":{"id":"rule456","name":"New Access Rule","desc":"A dummy access rule","blacklist_enabled":true,"whitelist_enabled":false}}`,
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
			var deserializedEvent events.Event
			if err := events.ParseEvent(jsonData, &deserializedEvent); err != nil {
				t.Fatalf("Failed to parse event: %v", err)
			}

			// Compare the original event with the deserialized event
			if deserializedEvent.Name != test.event.Name || deserializedEvent.Timestamp != test.event.Timestamp {
				t.Fatalf("Deserialized event does not match original.\nGot:  %+v\nWant: %+v", deserializedEvent, test.event)
			}

			switch data := deserializedEvent.Data.(type) {
			case *events.BlacklistedIPBlockedEvent:
				originalData, ok := test.event.Data.(*events.BlacklistedIPBlockedEvent)
				if !ok || *data != *originalData {
					t.Fatalf("Deserialized BlacklistedIPBlockedEvent does not match original.\nGot:  %+v\nWant: %+v", data, originalData)
				}
			case *events.AccessRuleCreatedEvent:
				originalData, ok := test.event.Data.(*events.AccessRuleCreatedEvent)
				if !ok || *data != *originalData {
					t.Fatalf("Deserialized AccessRuleCreatedEvent does not match original.\nGot:  %+v\nWant: %+v", data, originalData)
				}
			case *events.BlacklistToggledEvent:
				originalData, ok := test.event.Data.(*events.BlacklistToggledEvent)
				if !ok || *data != *originalData {
					t.Fatalf("Deserialized BlacklistToggledEvent does not match original.\nGot:  %+v\nWant: %+v", data, originalData)
				}
			default:
				t.Fatalf("Unknown event type: %T", data)
			}
		})
	}
}
