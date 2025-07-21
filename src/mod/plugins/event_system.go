package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/info/logger"
	zoraxyPlugin "imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

// eventManager manages event subscriptions and dispatching
type eventManager struct {
	subscriptions map[zoraxyPlugin.EventName][]string    // EventType -> []PluginID
	pluginLookup  func(pluginID string) (*Plugin, error) // Function to get plugin by ID
	logger        *logger.Logger                         // Logger for the event manager
	mutex         sync.RWMutex                           // Mutex for concurrent access
}

var (
	// EventSystem is the singleton instance of the event manager
	EventSystem *eventManager
	once        sync.Once
)

// InitEventManager initializes the event manager with the plugin manager
func InitEventManager(pluginLookup func(pluginID string) (*Plugin, error), logger *logger.Logger) {
	once.Do(func() {
		EventSystem = &eventManager{
			subscriptions: make(map[zoraxyPlugin.EventName][]string),
			pluginLookup:  pluginLookup,
			logger:        logger,
		}
	})
}

// Subscribe adds a plugin to the subscription list for an event type
func (em *eventManager) Subscribe(pluginID string, eventType zoraxyPlugin.EventName) error {
	em.mutex.Lock()
	defer em.mutex.Unlock()

	if _, exists := em.subscriptions[eventType]; !exists {
		em.subscriptions[eventType] = []string{}
	}

	// Check if already subscribed
	for _, id := range em.subscriptions[eventType] {
		if id == pluginID {
			return nil // Already subscribed
		}
	}

	em.subscriptions[eventType] = append(em.subscriptions[eventType], pluginID)
	return nil
}

// Unsubscribe removes a plugin from the subscription list for an event type

func (em *eventManager) Unsubscribe(pluginID string, eventType zoraxyPlugin.EventName) error {
	em.mutex.Lock()
	defer em.mutex.Unlock()

	if subscribers, exists := em.subscriptions[eventType]; exists {
		for i, id := range subscribers {
			if id == pluginID {
				// Remove from slice
				em.subscriptions[eventType] = append(subscribers[:i], subscribers[i+1:]...)
				break
			}
		}
	}

	return nil
}

// UnsubscribeAll removes a plugin from all event subscriptions
func (em *eventManager) UnsubscribeAll(pluginID string) error {
	em.mutex.Lock()
	defer em.mutex.Unlock()

	for eventType, subscribers := range em.subscriptions {
		for i, id := range subscribers {
			if id == pluginID {
				em.subscriptions[eventType] = append(subscribers[:i], subscribers[i+1:]...)
				break
			}
		}
	}

	return nil
}

// Emit dispatches an event to all subscribed plugins
func (em *eventManager) Emit(payload zoraxyPlugin.EventPayload) error {
	eventName := payload.GetName()

	em.mutex.RLock()
	subscribers, exists := em.subscriptions[eventName]
	em.mutex.RUnlock()

	if !exists || len(subscribers) == 0 {
		return nil // No subscribers
	}

	// Create the event
	event := zoraxyPlugin.Event{
		Name:      eventName,
		Timestamp: time.Now().Unix(),
		Data:      payload,
	}

	// Dispatch to all subscribers asynchronously
	for _, pluginID := range subscribers {
		go em.dispatchToPlugin(pluginID, event)
	}

	return nil
}

// dispatchToPlugin sends an event to a specific plugin
func (em *eventManager) dispatchToPlugin(pluginID string, event zoraxyPlugin.Event) {
	plugin, err := em.pluginLookup(pluginID)
	if err != nil {
		em.logger.PrintAndLog("event-system", "Failed to get plugin for event dispatch: "+pluginID, err)
		return
	}

	if !plugin.Enabled || plugin.AssignedPort == 0 {
		// Plugin is not running, skip
		return
	}

	subscriptionPath := plugin.Spec.SubscriptionPath
	if subscriptionPath == "" {
		// No subscription path configured, skip
		return
	}
	if !strings.HasPrefix(subscriptionPath, "/") {
		subscriptionPath = "/" + subscriptionPath
	}

	// Prepare the URL
	url := fmt.Sprintf("http://127.0.0.1:%d%s/%s", plugin.AssignedPort, subscriptionPath, event.Name)

	// Marshal the event to JSON
	eventData, err := json.Marshal(event)
	if err != nil {
		em.logger.PrintAndLog("event-system", "Failed to marshal event for plugin "+pluginID, err)
		return
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(eventData))
	if err != nil {
		em.logger.PrintAndLog("event-system", "Failed to create HTTP request for plugin "+pluginID, err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Zoraxy-Event-Type", string(event.Name))

	// Send the request with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		em.logger.PrintAndLog("event-system", "Failed to send event to plugin "+pluginID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody := fmt.Errorf("no response body")
		if resp.ContentLength > 0 {
			buffer := bytes.NewBuffer(make([]byte, 0, resp.ContentLength))
			_, respErr := buffer.ReadFrom(resp.Body)
			if respErr != nil {
				respBody = fmt.Errorf("failed to read response body: %v", respErr)
			} else {
				respBody = fmt.Errorf("response body: %s", buffer.String())
			}
		}

		em.logger.PrintAndLog("event-system", fmt.Sprintf("Plugin %s returned non-200 status for event `%s`: %s", pluginID, event.Name, resp.Status), respBody)
	}
}
