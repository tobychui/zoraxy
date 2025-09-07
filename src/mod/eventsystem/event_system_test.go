package eventsystem

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"imuslab.com/zoraxy/mod/info/logger"
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

type TestListener struct {
	id             ListenerID
	receivedEvents chan events.Event
}

func (tl *TestListener) GetID() ListenerID {
	return tl.id
}

func (tl *TestListener) Notify(event events.Event) error {
	tl.receivedEvents <- event
	return nil
}

func TestEventEmission(t *testing.T) {
	logger, err := logger.NewFmtLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	em := eventManager{
		subscriptions: make(map[events.EventName][]ListenerID),
		subscribers:   make(map[ListenerID]Listener),
		logger:        logger,
	}

	// Create a test listener
	listenerID := ListenerID("testListener")
	testListener := &TestListener{
		id:             listenerID,
		receivedEvents: make(chan events.Event, 10),
	}

	// Register the listener for BlacklistedIPBlocked events
	if err := em.RegisterSubscriberToEvent(testListener, events.EventBlacklistedIPBlocked); err != nil {
		t.Fatalf("Failed to register subscriber: %v", err)
	}

	// Emit a BlacklistedIPBlocked event
	testEvent := &events.BlacklistedIPBlockedEvent{
		IP:           "192.168.1.1",
		Comment:      "Malicious activity detected",
		RequestedURL: "http://mysite.com/admin",
		Hostname:     "mysite.com",
		UserAgent:    "BadBot/1.0",
		Method:       "GET",
	}
	em.Emit(testEvent)

	// Verify that the listener received the event
	select {
	case receivedEvent := <-testListener.receivedEvents:
		if receivedEvent.Name != events.EventBlacklistedIPBlocked {
			t.Fatalf("Unexpected event received by listener.\nGot:  %+v\nWant: %+v", receivedEvent, testEvent)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for event to be received by listener")
	}
	// Unregister the listener
	if err := em.UnregisterSubscriber(listenerID); err != nil {
		t.Fatalf("Failed to unregister subscriber: %v", err)
	}
}

func TestEventEmissionToSpecificListener(t *testing.T) {
	logger, err := logger.NewFmtLogger()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	em := eventManager{
		subscriptions: make(map[events.EventName][]ListenerID),
		subscribers:   make(map[ListenerID]Listener),
		logger:        logger,
	}

	// Create a few test listeners
	// listener1 and listener2 are plugins that will send events to each other
	// moderator is a plugin that is subscribed to EventCustom, so it will receive all custom events
	// otherListener is neither subscribed to EventCustom nor a recipient of any custom event, so it should not receive any events
	listener1ID := ListenerID("pluginA")
	listener1 := &TestListener{
		id:             listener1ID,
		receivedEvents: make(chan events.Event, 10),
	}
	em.RegisterSubscriberToEvent(listener1, events.EventAccessRuleCreated) // We need to register to some event to be a valid subscriber

	listener2ID := ListenerID("pluginB")
	listener2 := &TestListener{
		id:             listener2ID,
		receivedEvents: make(chan events.Event, 10),
	}
	em.RegisterSubscriberToEvent(listener2, events.EventAccessRuleCreated) // We need to register to some event to be a valid subscriber

	moderatorID := ListenerID("moderator")
	moderator := &TestListener{
		id:             moderatorID,
		receivedEvents: make(chan events.Event, 10),
	}
	em.RegisterSubscriberToEvent(moderator, events.EventCustom)

	otherListenerID := ListenerID("pluginD")
	otherListener := &TestListener{
		id:             otherListenerID,
		receivedEvents: make(chan events.Event, 10),
	}
	em.RegisterSubscriberToEvent(otherListener, events.EventAccessRuleCreated) // We need to register to some event to be a valid subscriber

	// Send a custom event from listener1 to listener2
	customEvent := &events.CustomEvent{
		SourcePlugin: "pluginA",
		Recipients:   []string{"pluginB"},
		Payload: map[string]interface{}{
			"message": "Hello from pluginA",
		},
	}
	em.EmitToSubscribersAnd([]ListenerID{listener2ID}, customEvent)

	// Verify that listener2 received the event
	select {
	case receivedEvent := <-listener2.receivedEvents:
		if receivedEvent.Name != events.EventCustom {
			t.Fatalf("Unexpected event received by listener2.\nGot:  %+v\nWant: %+v", receivedEvent, customEvent)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for event to be received by listener2")
	}
	// Verify that listener1 did not receive the event
	select {
	case <-listener1.receivedEvents:
		t.Fatal("Listener1 should not have received any events")
	case <-time.After(500 * time.Millisecond):
		// No event received, as expected
	}

	// send a custom event from listener2 to listener1
	customEvent2 := &events.CustomEvent{
		SourcePlugin: "pluginB",
		Recipients:   []string{"pluginA"},
		Payload: map[string]interface{}{
			"message": "Hello from pluginB",
		},
	}
	em.EmitToSubscribersAnd([]ListenerID{listener1ID}, customEvent2)

	// Verify that listener1 received the event
	select {
	case receivedEvent := <-listener1.receivedEvents:
		if receivedEvent.Name != events.EventCustom {
			t.Fatalf("Unexpected event received by listener1.\nGot:  %+v\nWant: %+v", receivedEvent, customEvent2)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for event to be received by listener1")
	}
	// Verify that listener2 did not receive any new event
	select {
	case <-listener2.receivedEvents:
		t.Fatal("Listener2 should not have received any new events")
	case <-time.After(500 * time.Millisecond):
		// No event received, as expected
	}

	// ensure the moderator received both events
	expectedMessagesSeen := map[string]bool{
		"Hello from pluginA": false,
		"Hello from pluginB": false,
	}
	for i := 0; i < 2; i++ {
		select {
		case receivedEvent := <-moderator.receivedEvents:
			if receivedEvent.Name != events.EventCustom {
				t.Fatalf("Unexpected event received by moderator.\nGot:  %+v\nWant: %+v", receivedEvent, customEvent)
			}
			data, ok := receivedEvent.Data.(*events.CustomEvent)
			if !ok {
				t.Fatalf("Unexpected data type in event received by moderator.\nGot:  %T\nWant: *events.CustomEvent", receivedEvent.Data)
			}
			message, ok := data.Payload["message"].(string)
			if !ok {
				t.Fatalf("Unexpected payload format in event received by moderator.\nGot:  %+v\nWant: map with 'message' key", data.Payload)
			}
			if alreadySeen, exists := expectedMessagesSeen[message]; exists {
				if alreadySeen {
					t.Fatalf("Duplicate message in event received by moderator: %s", message)
				}
				expectedMessagesSeen[message] = true
			} else {
				t.Fatalf("Unexpected message in event received by moderator: %s", message)
			}
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for event to be received by moderator")
		}
	}
	for msg, seen := range expectedMessagesSeen {
		if !seen {
			t.Fatalf("Moderator did not receive expected message: %s", msg)
		}
	}

	// Ensure that the events were not seen by the other listener
	select {
	case <-otherListener.receivedEvents:
		t.Fatal("otherListener should not have received any events")
	case <-time.After(500 * time.Millisecond):
		// No event received, as expected
	}
}
