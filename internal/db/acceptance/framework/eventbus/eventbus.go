// Package eventbus provides a thread-safe async event dispatcher with panic isolation.
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
	sub    interfaces.EventSubscriber
	ch     chan types.Event
	closed sync.Once
}

func (w *subscriberWrapper) close() {
	w.closed.Do(func() { close(w.ch) })
}

// EventBus dispatches lifecycle events to subscribers.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string]*subscriberWrapper
	logger      *logging.Logger
	dispatchWG  sync.WaitGroup
	subWG       sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	running     bool
}

// NewEventBus creates a bus.
func NewEventBus(log *logging.Logger) *EventBus {
	return &EventBus{
		subscribers: make(map[string]*subscriberWrapper),
		logger:      log,
	}
}

// Start enables publishing.
func (b *EventBus) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running {
		return nil
	}
	b.ctx, b.cancel = context.WithCancel(ctx)
	b.running = true
	b.logger.Info("ATF EventBus started")
	return nil
}

// Stop drains in-flight dispatches and subscriber loops.
func (b *EventBus) Stop() error {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return nil
	}
	b.running = false
	if b.cancel != nil {
		b.cancel()
	}
	subs := make([]*subscriberWrapper, 0, len(b.subscribers))
	for _, s := range b.subscribers {
		subs = append(subs, s)
	}
	b.subscribers = make(map[string]*subscriberWrapper)
	b.mu.Unlock()

	b.dispatchWG.Wait()
	for _, s := range subs {
		s.close()
	}
	b.subWG.Wait()
	b.logger.Info("ATF EventBus stopped")
	return nil
}

// Subscribe registers a subscriber. Bus must be started.
func (b *EventBus) Subscribe(sub interfaces.EventSubscriber) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.running {
		return fmt.Errorf("eventbus: not started")
	}
	name := sub.Name()
	if _, exists := b.subscribers[name]; exists {
		return fmt.Errorf("subscriber %s already registered", name)
	}
	w := &subscriberWrapper{
		sub: sub,
		ch:  make(chan types.Event, 64),
	}
	b.subscribers[name] = w
	b.subWG.Add(1)
	go b.subscriberLoop(w)
	return nil
}

// Unsubscribe removes a subscriber and waits for its loop to exit.
func (b *EventBus) Unsubscribe(sub interfaces.EventSubscriber) error {
	b.mu.Lock()
	name := sub.Name()
	w, exists := b.subscribers[name]
	if !exists {
		b.mu.Unlock()
		return fmt.Errorf("subscriber %s not found", name)
	}
	delete(b.subscribers, name)
	b.mu.Unlock()
	w.close()
	return nil
}

// Publish enqueues an event for all subscribers (non-blocking with drop on full/cancel).
func (b *EventBus) Publish(ctx context.Context, et types.EventType, payload any) {
	b.mu.RLock()
	if !b.running {
		b.mu.RUnlock()
		return
	}
	subs := make([]*subscriberWrapper, 0, len(b.subscribers))
	for _, w := range b.subscribers {
		subs = append(subs, w)
	}
	busCtx := b.ctx
	b.mu.RUnlock()

	event := types.Event{Type: et, Timestamp: time.Now(), Payload: payload}
	for _, w := range subs {
		b.dispatchWG.Add(1)
		go func(w *subscriberWrapper, ev types.Event) {
			defer b.dispatchWG.Done()
			select {
			case w.ch <- ev:
			case <-ctx.Done():
				b.logger.Warn("event drop (caller ctx) subscriber=%s", w.sub.Name())
			case <-busCtx.Done():
				b.logger.Warn("event drop (bus stopped) subscriber=%s", w.sub.Name())
			}
		}(w, event)
	}
}

func (b *EventBus) subscriberLoop(w *subscriberWrapper) {
	defer b.subWG.Done()
	for event := range w.ch {
		b.invokeSubscriber(w.sub, event)
	}
}

func (b *EventBus) invokeSubscriber(sub interfaces.EventSubscriber, ev types.Event) {
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("panic in subscriber %s: %v", sub.Name(), r)
		}
	}()
	ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
	defer cancel()
	if err := sub.OnEvent(ctx, ev); err != nil {
		b.logger.Error("subscriber %s error: %v", sub.Name(), err)
	}
}
