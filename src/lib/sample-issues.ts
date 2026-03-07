export type IssueRecord = {
  id: string;
  raw: string;
  tags: string[];
  createdBy: string;
  createdAt: string;
};

export const INITIAL_ISSUES: IssueRecord[] = [
  {
    id: "sample-1",
    raw: "We should add dark mode support across the whole app",
    tags: ["feature", "ui"],
    createdBy: "Alice",
    createdAt: "2026-03-06T21:15:00Z",
  },
  {
    id: "sample-2",
    raw: "The onboarding flow feels clunky — can we just ask for an email and skip the rest until later?",
    tags: ["ux", "improvement", "onboarding"],
    createdBy: "Bob",
    createdAt: "2026-03-06T19:00:00Z",
  },
  {
    id: "sample-3",
    raw: `TypeError: Cannot read properties of undefined (reading 'map')
    at ProjectList (./src/components/ProjectList.tsx:14:22)
    at renderWithHooks (./node_modules/react-dom/cjs/react-dom.development.js:16305:18)
    at mountIndeterminateComponent (./node_modules/react-dom/cjs/react-dom.development.js:20074:13)
    at beginWork (./node_modules/react-dom/cjs/react-dom.development.js:21587:16)`,
    tags: ["bug", "crash", "frontend"],
    createdBy: "Charlie",
    createdAt: "2026-03-06T17:00:00Z",
  },
  {
    id: "sample-4",
    raw: "Search is way too slow on large workspaces. Takes 4+ seconds to return results. Probably need to index or debounce the input.",
    tags: ["bug", "performance", "search"],
    createdBy: "Alice",
    createdAt: "2026-03-06T14:00:00Z",
  },
  {
    id: "sample-5",
    raw: "idea: what if issues could link to each other automatically when they mention similar things?",
    tags: ["idea", "feature"],
    createdBy: "Diana",
    createdAt: "2026-03-05T20:00:00Z",
  },
  {
    id: "sample-6",
    raw: `Customer reported: "I clicked export and nothing happened. Tried 3 times. Using Safari on iPad."`,
    tags: ["bug", "export", "safari"],
    createdBy: "Bob",
    createdAt: "2026-03-05T18:00:00Z",
  },
];
