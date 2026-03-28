package issues

import "context"

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
