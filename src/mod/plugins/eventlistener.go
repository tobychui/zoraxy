// implements `eventsystem.Listener` for Plugin

package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/eventsystem"
	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin/events"
)

func (p *Plugin) GetID() eventsystem.ListenerID {
	return eventsystem.ListenerID(p.Spec.ID)
}

// Send an event to the plugin
func (p *Plugin) Notify(event events.Event) error {
	// Handle the event notification
	if !p.Enabled || p.AssignedPort == 0 {
		return fmt.Errorf("plugin %s is not running", p.Spec.ID)
	}

	subscriptionPath := p.Spec.SubscriptionPath
	if subscriptionPath == "" {
		return fmt.Errorf("plugin %s has no subscription path configured", p.Spec.ID)
	}

	if !strings.HasPrefix(subscriptionPath, "/") {
		subscriptionPath = "/" + subscriptionPath
	}
	subscriptionPath = strings.TrimSuffix(subscriptionPath, "/")

	// Prepare the URL
	url := fmt.Sprintf("http://127.0.0.1:%d%s/%s", p.AssignedPort, subscriptionPath, event.Name)

	// Marshal the event to JSON
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(eventData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Zoraxy-Event-Type", string(event.Name))

	// Send the request with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return errors.New("Failed to send event: " + err.Error())
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

		return fmt.Errorf("plugin %s returned non-200 status for event `%s` (%s): %w", p.Spec.ID, event.Name, resp.Status, respBody)
	}

	return nil
}
