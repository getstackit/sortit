package issues

import "time"

// FixtureIssues returns a set of sample issues for use in tests.
func FixtureIssues() []Issue {
	return []Issue{
		{
			ID:        "sample-1",
			Raw:       "We should add dark mode support across the whole app",
			Tags:      []string{"feature", "ui"},
			CreatedBy: "Alice",
			CreatedAt: mustParseTimestamp("2026-03-06T21:15:00Z"),
		},
		{
			ID:        "sample-2",
			Raw:       "The onboarding flow feels clunky — can we just ask for an email and skip the rest until later?",
			Tags:      []string{"ux", "improvement", "onboarding"},
			CreatedBy: "Bob",
			CreatedAt: mustParseTimestamp("2026-03-06T19:00:00Z"),
		},
		{
			ID:        "sample-3",
			Raw:       "TypeError: Cannot read properties of undefined (reading 'map')\n    at ProjectList (./src/components/ProjectList.tsx:14:22)\n    at renderWithHooks (./node_modules/react-dom/cjs/react-dom.development.js:16305:18)\n    at mountIndeterminateComponent (./node_modules/react-dom/cjs/react-dom.development.js:20074:13)\n    at beginWork (./node_modules/react-dom/cjs/react-dom.development.js:21587:16)",
			Tags:      []string{"bug", "crash", "frontend"},
			CreatedBy: "Charlie",
			CreatedAt: mustParseTimestamp("2026-03-06T17:00:00Z"),
		},
		{
			ID:        "sample-4",
			Raw:       "Search is way too slow on large workspaces. Takes 4+ seconds to return results. Probably need to index or debounce the input.",
			Tags:      []string{"bug", "performance", "search"},
			CreatedBy: "Alice",
			CreatedAt: mustParseTimestamp("2026-03-06T14:00:00Z"),
		},
		{
			ID:        "sample-5",
			Raw:       "idea: what if issues could link to each other automatically when they mention similar things?",
			Tags:      []string{"idea", "feature"},
			CreatedBy: "Diana",
			CreatedAt: mustParseTimestamp("2026-03-05T20:00:00Z"),
		},
		{
			ID:        "sample-6",
			Raw:       "Customer reported: \"I clicked export and nothing happened. Tried 3 times. Using Safari on iPad.\"",
			Tags:      []string{"bug", "export", "safari"},
			CreatedBy: "Bob",
			CreatedAt: mustParseTimestamp("2026-03-05T18:00:00Z"),
		},
	}
}

func mustParseTimestamp(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}
