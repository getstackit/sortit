package issues

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *InMemoryStore) RecordEvent(_ context.Context, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append([]Event{event}, s.events...)
	return nil
}

func (s *InMemoryStore) ListEvents(_ context.Context, limit int, cursor string, kind string) ([]Event, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 40
	}

	kind = strings.TrimSpace(kind)
	cursorTime, cursorID, hasCursor := decodeEventCursor(cursor)

	filtered := make([]Event, 0, len(s.events))
	for _, event := range s.events {
		if kind != "" && !strings.EqualFold(event.Kind, kind) {
			continue
		}
		if hasCursor {
			if event.CreatedAt.After(cursorTime) {
				continue
			}
			if event.CreatedAt.Equal(cursorTime) && event.ID >= cursorID {
				continue
			}
		}
		filtered = append(filtered, event)
	}

	if len(filtered) > limit {
		result := make([]Event, limit)
		copy(result, filtered[:limit])
		last := result[len(result)-1]
		return result, fmt.Sprintf("%d|%s", last.CreatedAt.UnixNano(), last.ID), nil
	}

	return filtered, "", nil
}

func (s *InMemoryStore) ListLifecycleEvents(_ context.Context, kinds []string, start, end time.Time) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(kinds) == 0 {
		return nil, nil
	}
	kindSet := make(map[string]struct{}, len(kinds))
	for _, k := range kinds {
		kindSet[strings.TrimSpace(k)] = struct{}{}
	}

	out := make([]Event, 0)
	for _, event := range s.events {
		if _, ok := kindSet[event.Kind]; !ok {
			continue
		}
		if event.CreatedAt.Before(start) || event.CreatedAt.After(end) {
			continue
		}
		if strings.TrimSpace(event.IssueID) == "" {
			continue
		}
		out = append(out, event)
	}
	return out, nil
}
