package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	plugin "aroz.org/zoraxy/plugins/plugin2plugin-comms-peer1/mod/zoraxy_plugin"
	"aroz.org/zoraxy/plugins/plugin2plugin-comms-peer1/mod/zoraxy_plugin/events"
)

type Message struct {
	Message string `json:"message"`
	Sent    bool   `json:"sent"`
}

var (
	// map of connected SSE clients
	messageHistory   []Message = make([]Message, 0)
	messageHistoryMu           = &sync.Mutex{}
	clients                    = make(map[chan *events.CustomEvent]struct{})
	clientsMu                  = &sync.Mutex{}
)

func sendMessageToPeer(config *plugin.ConfigureSpec, message string) error {
	// build the request payload
	event := events.CustomEvent{
		SourcePlugin: PLUGIN_ID,
		Recipients:   []string{PEER_ID},
		Payload:      map[string]any{"message": message},
	}

	// Make an API call to the peer plugin's endpoint
	client := &http.Client{}
	apiURL := fmt.Sprintf("http://localhost:%d/plugin/event/emit", config.ZoraxyPort)
	payload := new(bytes.Buffer)
	if err := json.NewEncoder(payload).Encode(event); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, apiURL, payload)
	if err != nil {
		return err
	}

	// Make sure to set the Authorization header
	req.Header.Set("Authorization", "Bearer "+config.APIKey) // Use the API key from the runtime config
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		response_body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Response Body: %s\n", string(response_body))
		return fmt.Errorf("failed to call the zoraxy API: %s, %v", resp.Status, string(response_body))
	}

	return nil
}

func handleSendMessage(config *plugin.ConfigureSpec, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the message body
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Failed to parse JSON body", http.StatusBadRequest)
		return
	}

	message := body.Message
	if message == "" {
		http.Error(w, "Message cannot be empty", http.StatusBadRequest)
		return
	}

	// send the message to the peer plugin
	err := sendMessageToPeer(config, message)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send message to peer: %v", err), http.StatusInternalServerError)
		return
	}

	// Log the sent message
	messageHistoryMu.Lock()
	messageHistory = append(messageHistory, Message{Message: message, Sent: true})
	messageHistoryMu.Unlock()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Message sent to peer successfully"))
}

func handleFetchMessageHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	messageHistoryMu.Lock()
	historyCopy := make([]Message, len(messageHistory))
	copy(historyCopy, messageHistory)
	messageHistoryMu.Unlock()

	resp := struct {
		Messages []Message `json:"messages"`
	}{
		Messages: historyCopy,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleReceivedEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var event events.Event

	// read the request body
	if r.Body == nil || r.ContentLength == 0 {
		http.Error(w, "Request body is empty", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	buffer := bytes.NewBuffer(make([]byte, 0, r.ContentLength))
	if _, err := buffer.ReadFrom(r.Body); err != nil {
		http.Error(w, fmt.Sprintf("Failed to read request body: %v", err), http.StatusBadRequest)
		return
	}

	// parse the event from the request body
	if err := events.ParseEvent(buffer.Bytes(), &event); err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse event: %v", err), http.StatusBadRequest)
		return
	}

	switch event.Name {
	case events.EventCustom:
		// downcast event.Data to CustomEvent
		customData, ok := event.Data.(*events.CustomEvent)
		if !ok {
			http.Error(w, "Invalid event data for CustomEvent", http.StatusBadRequest)
			return
		}
		// Log the received message
		messageHistoryMu.Lock()
		if msg, exists := customData.Payload["message"].(string); exists {
			messageHistory = append(messageHistory, Message{Message: msg, Sent: false})
		}
		messageHistoryMu.Unlock()

		// Broadcast to all connected SSE clients
		broadcastMessage(customData)

		// Respond to the sender
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Event received successfully"))
		// For demonstration, print the message to the console
		fmt.Printf("Received message from plugin %s: %v\n", customData.SourcePlugin, customData.Payload["message"])
	default:
		http.Error(w, fmt.Sprintf("Unhandled event type: %s", event.Name), http.StatusBadRequest)
		return
	}
}

// SSE handler
func handleSSE(w http.ResponseWriter, r *http.Request) {
	fmt.Println("SSE connection established")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	eventChan := make(chan *events.CustomEvent)
	clientsMu.Lock()
	clients[eventChan] = struct{}{}
	clientsMu.Unlock()

	defer func() {
		clientsMu.Lock()
		delete(clients, eventChan)
		clientsMu.Unlock()
		close(eventChan)
	}()

	// Send events as they arrive
	for event := range eventChan {
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}

// Broadcast to all clients
func broadcastMessage(message *events.CustomEvent) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for ch := range clients {
		select {
		case ch <- message:
		default:
			// If the client is not listening, skip
		}
	}
}
