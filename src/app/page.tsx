"use client";

import { useState } from "react";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import {
  SidebarInset,
  SidebarProvider,
} from "@/components/ui/sidebar";

const TAG_COLORS: Record<string, string> = {
  bug: "bg-red-100 text-red-700",
  crash: "bg-red-100 text-red-700",
  feature: "bg-purple-100 text-purple-700",
  idea: "bg-purple-100 text-purple-700",
  improvement: "bg-green-100 text-green-700",
  ui: "bg-blue-100 text-blue-700",
  ux: "bg-blue-100 text-blue-700",
  frontend: "bg-blue-100 text-blue-700",
  performance: "bg-amber-100 text-amber-700",
  safari: "bg-amber-100 text-amber-700",
};

const FALLBACK_COLOR = "bg-slate-100 text-slate-600";

const INITIAL_ISSUES: Issue[] = [
  {
    id: "sample-1",
    raw: "We should add dark mode support across the whole app",
    tags: ["feature", "ui"],
    createdBy: "Alice",
    createdAt: new Date("2026-03-06T21:15:00Z"),
  },
  {
    id: "sample-2",
    raw: "The onboarding flow feels clunky — can we just ask for an email and skip the rest until later?",
    tags: ["ux", "improvement", "onboarding"],
    createdBy: "Bob",
    createdAt: new Date("2026-03-06T19:00:00Z"),
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
    createdAt: new Date("2026-03-06T17:00:00Z"),
  },
  {
    id: "sample-4",
    raw: "Search is way too slow on large workspaces. Takes 4+ seconds to return results. Probably need to index or debounce the input.",
    tags: ["bug", "performance", "search"],
    createdBy: "Alice",
    createdAt: new Date("2026-03-06T14:00:00Z"),
  },
  {
    id: "sample-5",
    raw: "idea: what if issues could link to each other automatically when they mention similar things?",
    tags: ["idea", "feature"],
    createdBy: "Diana",
    createdAt: new Date("2026-03-05T20:00:00Z"),
  },
  {
    id: "sample-6",
    raw: `Customer reported: "I clicked export and nothing happened. Tried 3 times. Using Safari on iPad."`,
    tags: ["bug", "export", "safari"],
    createdBy: "Bob",
    createdAt: new Date("2026-03-05T18:00:00Z"),
  },
];

function tagColor(label: string) {
  return TAG_COLORS[label] ?? FALLBACK_COLOR;
}

type Issue = {
  id: string;
  raw: string;
  tags: string[];
  createdBy: string;
  createdAt: Date;
};

function timeAgo(date: Date) {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return date.toLocaleDateString();
}

export default function Home() {
  const [issues, setIssues] = useState<Issue[]>(INITIAL_ISSUES);

  function handleSubmit(text: string) {
    setIssues((prev) => [
      {
        id: crypto.randomUUID(),
        raw: text,
        tags: [],
        createdBy: "You",
        createdAt: new Date(),
      },
      ...prev,
    ]);
  }

  const looksLikeCode = (text: string) =>
    text.includes("Error") ||
    text.includes("at ") ||
    text.includes("=>") ||
    text.includes("function") ||
    text.includes("{") ||
    /^\s*(import|const|let|var|def|class)\b/m.test(text);

  return (
    <SidebarProvider
      style={
        {
          "--sidebar-width": "calc(var(--spacing) * 72)",
          "--header-height": "calc(var(--spacing) * 12)",
        } as React.CSSProperties
      }
    >
      <AppSidebar
        variant="inset"
        things={issues.map((issue) => ({
          id: issue.id,
          title: issue.raw.length > 60 ? issue.raw.slice(0, 60) + "…" : issue.raw,
        }))}
      />
      <SidebarInset>
        <SiteHeader issueCount={issues.length} onSubmit={handleSubmit} />
        <div className="flex flex-1 flex-col">
          <div className="@container/main flex flex-1 flex-col gap-2">
            <div className="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
              {issues.length === 0 && (
                <div className="flex flex-col items-center gap-3 py-20 text-center">
                  <div className="text-4xl opacity-20">~</div>
                  <p className="text-sm text-muted-foreground/60">
                    Nothing here yet. Drop something in.
                  </p>
                </div>
              )}

              {issues.length > 0 && (
                <div className="space-y-2 px-4 lg:px-6">
                  {issues.map((issue) => (
                    <div
                      key={issue.id}
                      className="group/card rounded-xl border border-border/60 bg-card p-5 transition-colors hover:border-border hover:bg-accent/30"
                    >
                      <p
                        className={`whitespace-pre-wrap leading-relaxed ${
                          looksLikeCode(issue.raw)
                            ? "font-mono text-[13px] text-foreground/80"
                            : "text-[15px]"
                        }`}
                      >
                        {issue.raw}
                      </p>
                      <div className="mt-3 flex items-center gap-2">
                        <div className="flex flex-wrap gap-1.5">
                          {issue.tags.map((tag) => (
                            <span
                              key={tag}
                              className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${tagColor(tag)}`}
                            >
                              {tag}
                            </span>
                          ))}
                        </div>
                        <span className="ml-auto text-[11px] tracking-wide text-muted-foreground/40 transition-colors group-hover/card:text-muted-foreground/60">
                          {issue.createdBy} &middot; {timeAgo(issue.createdAt)}
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      </SidebarInset>
    </SidebarProvider>
  );
}
