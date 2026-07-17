// Package eventbus implements a thread-safe, asynchronous event bus with context propagation,
// error isolation, and graceful shutdown. It decouples execution orchestrators from telemetry,
// loggers, and artifact archivers.
//
// Dependency Rules:
// - Imports: interfaces, types, logging.
package eventbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/interfaces"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

type subscriberWrapper struct {
	sub interfaces.EventSubscriber
	ch  chan types.Event
}

// EventBus dispatches lifecycle events asynchronously to registered subscribers.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string]*subscriberWrapper
	logger      *logging.Logger
	wg          sync.WaitGroup
	subWg       sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	running     bool
}

// NewEventBus creates a new EventBus instance.
func NewEventBus(log *logging.Logger) *EventBus {
	return &EventBus{
		subscribers: make(map[string]*subscriberWrapper),
		logger:      log,
	}
}

// Start launches the event bus processing loop.
func (b *EventBus) Start(ctx context.Context) error {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return nil
	}
	b.ctx, b.cancel = context.WithCancel(ctx)
	b.running = true
	b.mu.Unlock()

	b.logger.Info("ATF EventBus started")
	return nil
}

// Stop gracefully shuts down the event bus, flushing pending events to subscribers.
func (b *EventBus) Stop() error {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return nil
	}
	b.running = false
	b.cancel()
	b.mu.Unlock()

	// Wait for any active asynchronous event dispatches to finish
	b.wg.Wait()

	b.mu.Lock()
	// Close all subscriber channels to signal shutdown
	for _, sub := range b.subscribers {
		close(sub.ch)
	}
	b.subscribers = make(map[string]*subscriberWrapper)
	b.mu.Unlock()

	// Wait for all subscriber loops to drain and exit
	b.subWg.Wait()

	b.logger.Info("ATF EventBus stopped cleanly")
	return nil
}

// Subscribe registers an event observer to receive future event dispatches.
func (b *EventBus) Subscribe(sub interfaces.EventSubscriber) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	name := sub.Name()
	if _, exists := b.subscribers[name]; exists {
		return fmt.Errorf("subscriber %s already registered", name)
	}

	wrapper := &subscriberWrapper{
		sub: sub,
		ch:  make(chan types.Event, 100), // Buffer to avoid blocking dispatch
	}

	b.subscribers[name] = wrapper
	b.subWg.Add(1)
	go b.subscriberLoop(wrapper)

	b.logger.Debug("Subscriber %s registered successfully", name)
	return nil
}

// Unsubscribe removes a registered observer.
func (b *EventBus) Unsubscribe(sub interfaces.EventSubscriber) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	name := sub.Name()
	wrapper, exists := b.subscribers[name]
	if !exists {
		return fmt.Errorf("subscriber %s not found", name)
	}

	delete(b.subscribers, name)
	close(wrapper.ch) // Signal subscriberLoop to exit

	b.logger.Debug("Subscriber %s unsubscribed", name)
	return nil
}

// Publish writes an event to all active subscriber queues asynchronously.
func (b *EventBus) Publish(ctx context.Context, et types.EventType, payload interface{}) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.running {
		b.logger.Warn("Attempted to publish event to stopped EventBus")
		return
	}

	event := types.Event{
		Type:      et,
		Timestamp: time.Now(),
		Payload:   payload,
	}

	// Dispatch asynchronously to prevent slow subscribers from blocking the runner
	for _, wrapper := range b.subscribers {
		b.wg.Add(1)
		go func(w *subscriberWrapper, ev types.Event) {
			defer b.wg.Done()
			select {
			case w.ch <- ev:
			case <-ctx.Done():
				b.logger.Error("Event dispatch timed out for subscriber: %s", w.sub.Name())
			case <-b.ctx.Done():
			}
		}(wrapper, event)
	}
}

// subscriberLoop drains incoming events from the channel and executes the callback.
func (b *EventBus) subscriberLoop(wrapper *subscriberWrapper) {
	defer b.subWg.Done()

	for event := range wrapper.ch {
		b.invokeSubscriber(wrapper.sub, event)
	}
}

// invokeSubscriber handles subscriber callback execution and isolates panics/errors.
func (b *EventBus) invokeSubscriber(sub interfaces.EventSubscriber, ev types.Event) {
	// Guard against panic inside the subscriber implementation
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("Panic recovered inside subscriber %s: %v", sub.Name(), r)
		}
	}()

	ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
	defer cancel()

	if err := sub.OnEvent(ctx, ev); err != nil {
		b.logger.Error("Subscriber %s failed to process event: %v", sub.Name(), err)
	}
}
