package issues

import (
	"context"
	"sync"
)

type EventListener func(context.Context, Event)

type EventPublisher interface {
	Publish(context.Context, Event)
	PublishOne(context.Context, Event)
}

type EventSubscriber interface {
	Subscribe(EventListener)
}

type EventBus interface {
	EventPublisher
	EventSubscriber
}

type InProcessEventBus struct {
	mu        sync.RWMutex
	listeners []EventListener
}

func NewEventBus() *InProcessEventBus {
	return &InProcessEventBus{}
}

func (b *InProcessEventBus) Subscribe(listener EventListener) {
	if b == nil || listener == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.listeners = append(b.listeners, listener)
}

func (b *InProcessEventBus) Publish(ctx context.Context, event Event) {
	if b == nil {
		return
	}

	b.mu.RLock()
	listeners := append([]EventListener(nil), b.listeners...)
	b.mu.RUnlock()

	for _, listener := range listeners {
		listener(ctx, event)
	}
}

func (b *InProcessEventBus) PublishOne(ctx context.Context, event Event) {
	b.Publish(ctx, event)
}
