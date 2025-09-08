package eventsystem

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"imuslab.com/zoraxy/mod/info/logger"

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

// EmitToSubscribersAnd dispatches an event to the specific listeners in addition to the events subscribers.
//
// The primary use-case of this function is for plugin-to-plugin communication
func (em *eventManager) EmitToSubscribersAnd(listenerIDs []ListenerID, payload events.EventPayload) {
	eventName := payload.GetName()

	if len(listenerIDs) == 0 {
		return // No listeners specified
	}

	// Create the event
	event := events.Event{
		Name:      eventName,
		Timestamp: time.Now().Unix(),
		UUID:      uuid.New().String(),
		Data:      payload,
	}

	// Dispatch to all specified listeners asynchronously
	em.emitTo(listenerIDs, event)

	// Also emit to all subscribers of the event as usual
	em.mutex.RLock()
	defer em.mutex.RUnlock()
	subscribers, exists := em.subscriptions[eventName]
	if !exists || len(subscribers) == 0 {
		return // No subscribers
	}
	em.emitTo(subscribers, event)
}

// Emit dispatches an event to all subscribed listeners
func (em *eventManager) Emit(payload events.EventPayload) {
	eventName := payload.GetName()

	em.mutex.RLock()
	defer em.mutex.RUnlock()
	subscribers, exists := em.subscriptions[eventName]

	if !exists || len(subscribers) == 0 {
		return // No subscribers
	}

	// Create the event
	event := events.Event{
		Name:      eventName,
		Timestamp: time.Now().Unix(),
		UUID:      uuid.New().String(),
		Data:      payload,
	}

	// Dispatch to all subscribers asynchronously
	em.emitTo(subscribers, event)
}

// Dispatch event to all specified listeners asynchronously
func (em *eventManager) emitTo(listenerIDs []ListenerID, event events.Event) {
	if len(listenerIDs) == 0 {
		return
	}

	// Dispatch to all specified listeners asynchronously
	em.mutex.RLock()
	defer em.mutex.RUnlock()
	listenersToUnregister := []ListenerID{}
	for _, listenerID := range listenerIDs {
		listener, exists := em.subscribers[listenerID]

		if !exists {
			em.logger.PrintAndLog("event-system", "Failed to get listener for event dispatch, removing "+string(listenerID)+" from subscriptions", nil)
			// Mark for removal
			listenersToUnregister = append(listenersToUnregister, listenerID)
			continue
		}

		go func(l Listener) {
			if err := l.Notify(event); err != nil {
				em.logger.PrintAndLog("event-system", "Failed to dispatch `"+string(event.Name)+"` event to listener "+string(listenerID), err)
			}
		}(listener)
	}

	// Unregister any listeners that no longer exist, asynchronously
	go func() {
		for _, id := range listenersToUnregister {
			em.UnregisterSubscriber(id)
		}
	}()
}
