package eventsystem

import (
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/info/logger"
	// "imuslab.com/zoraxy/mod/plugins"

	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin/events"
)

type ListenerID string
type Listener interface {
	Notify(event events.Event) error
	GetID() ListenerID
}

// eventManager manages event subscriptions and dispatching events to listeners
type eventManager struct {
	subscriptions map[events.EventName][]ListenerID // EventType -> []Subscriber, tracks which events each listener is subscribed to
	subscribers   map[ListenerID]Listener           // ListenerID -> Listener, tracks all registered listeners
	logger        *logger.Logger                    // Logger for the event manager
	mutex         sync.RWMutex                      // Mutex for concurrent access
}

var (
	// Publisher is the singleton instance of the event manager
	Publisher *eventManager
	once      sync.Once
)

// InitEventSystem initializes the event manager with the plugin manager
func InitEventSystem(logger *logger.Logger) {
	once.Do(func() {
		Publisher = &eventManager{
			subscriptions: make(map[events.EventName][]ListenerID),
			subscribers:   make(map[ListenerID]Listener),
			logger:        logger,
		}
	})
}

// RegisterSubscriberToEvent adds a listener to the subscription list for an event type
func (em *eventManager) RegisterSubscriberToEvent(subscriber Listener, eventType events.EventName) error {
	em.mutex.Lock()
	defer em.mutex.Unlock()

	if _, exists := em.subscriptions[eventType]; !exists {
		em.subscriptions[eventType] = []ListenerID{}
	}

	// Register the listener if not already registered
	listenerID := subscriber.GetID()
	em.subscribers[listenerID] = subscriber

	// Check if already subscribed to the event
	for _, id := range em.subscriptions[eventType] {
		if id == listenerID {
			return nil // Already subscribed
		}
	}

	// Register the listener to the event
	em.subscriptions[eventType] = append(em.subscriptions[eventType], listenerID)

	return nil
}

// Deregister removes a listener from all event subscriptions, and
// also removes the listener from the list of subscribers.
func (em *eventManager) UnregisterSubscriber(listenerID ListenerID) error {
	em.mutex.Lock()
	defer em.mutex.Unlock()

	for eventType, subscribers := range em.subscriptions {
		for i, id := range subscribers {
			if id == listenerID {
				em.subscriptions[eventType] = append(subscribers[:i], subscribers[i+1:]...)
				break
			}
		}
	}
	delete(em.subscribers, listenerID)

	return nil
}

// Emit dispatches an event to all subscribed listeners
func (em *eventManager) Emit(payload events.EventPayload) error {
	eventName := payload.GetName()

	em.mutex.RLock()
	defer em.mutex.RUnlock()
	subscribers, exists := em.subscriptions[eventName]

	if !exists || len(subscribers) == 0 {
		return nil // No subscribers
	}

	// Create the event
	event := events.Event{
		Name:      eventName,
		Timestamp: time.Now().Unix(),
		Data:      payload,
	}

	// Dispatch to all subscribers asynchronously
	for _, listenerID := range subscribers {
		listener, exists := em.subscribers[listenerID]

		if !exists {
			em.logger.PrintAndLog("event-system", "Failed to get listener for event dispatch, removing "+string(listenerID)+" from subscriptions", nil)
			continue
		}

		go func(l Listener) {
			if err := l.Notify(event); err != nil {
				em.logger.PrintAndLog("event-system", "Failed to dispatch `"+string(event.Name)+"` event to listener "+string(listenerID), err)
			}
		}(listener)
	}

	return nil
}
