package regions

import (
	"fmt"
	"time"

	"sortit/internal/domain"
)

// DefaultWindowLabel is the window applied when callers do not specify one.
const DefaultWindowLabel = "30d"

// SupportedWindowLabels lists the windows accepted by ParseTimeWindow,
// in display order from narrowest to widest.
var SupportedWindowLabels = []string{"7d", "30d", "90d", "180d", "365d", "all"}

// ParseTimeWindow converts a label like "30d" or "all" into a TimeWindow
// ending at now. Returns an error if the label is unrecognized; "all" maps
// to a zero Start so callers can treat it as unbounded.
func ParseTimeWindow(label string, now time.Time) (domain.TimeWindow, error) {
	switch label {
	case "7d":
		return domain.TimeWindow{Label: label, Start: now.Add(-7 * 24 * time.Hour), End: now}, nil
	case "30d":
		return domain.TimeWindow{Label: label, Start: now.Add(-30 * 24 * time.Hour), End: now}, nil
	case "90d":
		return domain.TimeWindow{Label: label, Start: now.Add(-90 * 24 * time.Hour), End: now}, nil
	case "180d":
		return domain.TimeWindow{Label: label, Start: now.Add(-180 * 24 * time.Hour), End: now}, nil
	case "365d":
		return domain.TimeWindow{Label: label, Start: now.Add(-365 * 24 * time.Hour), End: now}, nil
	case "all":
		return domain.TimeWindow{Label: label, End: now}, nil
	}
	return domain.TimeWindow{}, fmt.Errorf("unknown window: %q", label)
}
