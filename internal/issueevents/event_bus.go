package issueevents

import (
	"context"
	"sync"

	"splat/internal/issues"
)

type EventListener = issues.EventListener

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

func (b *InProcessEventBus) Publish(ctx context.Context, event issues.Event) {
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

func (b *InProcessEventBus) PublishOne(ctx context.Context, event issues.Event) {
	b.Publish(ctx, event)
}
