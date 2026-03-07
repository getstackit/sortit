"use client";

import { useEffect, useState } from "react";
import { AppShell, AppShellToggle } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { fetchIssues } from "@/lib/issues";
import { apiURL } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type TagScore = {
  tag: string;
  relevance: number;
  suggested?: boolean;
  description?: string;
};

type ModelInfo = {
  provider: string;
  model: string;
};

type IssueAnalysis = {
  tags: TagScore[];
  embedding: {
    dimensions: number;
    preview: number[];
    chunkCount: number;
    estimatedTokenCount: number;
    pooledFromChunks: boolean;
  };
  tagger: ModelInfo;
  embedder: ModelInfo;
};

type DebugIssueStoreResponse = {
  issueCount: number;
};

const SECTION_LINKS = [
  { id: "sandbox", title: "Sandbox" },
  { id: "prompt", title: "Prompt" },
  { id: "tags", title: "Tags" },
  { id: "embedding", title: "Embedding" },
  { id: "json", title: "JSON" },
];

const DEFAULT_TEXT = `Safari export crashes when I try to download a PDF from the issue detail view on iPad. It hangs for a second, then the sheet disappears and nothing is saved.`;

function formatFloat(value: number) {
  return value.toFixed(3);
}

async function parseAnalyzeResponse(response: Response) {
  const raw = await response.text();
  const contentType = response.headers.get("content-type") ?? "";
  const isJSON = contentType.includes("application/json");

  if (isJSON) {
    return JSON.parse(raw) as IssueAnalysis | { error?: string };
  }

  if (!response.ok) {
    throw new Error(raw.trim() || `Request failed with ${response.status}`);
  }

  throw new Error("Backend returned a non-JSON response.");
}

export default function DebugPage() {
  const [text, setText] = useState(DEFAULT_TEXT);
  const [tags, setTags] = useState("");
  const [result, setResult] = useState<IssueAnalysis | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [issueCount, setIssueCount] = useState<number | null>(null);
  const [sandboxLoading, setSandboxLoading] = useState(false);
  const [sandboxError, setSandboxError] = useState<string | null>(null);

  const topTags = result?.tags.slice(0, 8) ?? [];
  const embeddingPreview = result?.embedding.preview ?? [];

  useEffect(() => {
    fetchIssues()
      .then((issues) => {
        setIssueCount(issues.length);
        setSandboxError(null);
      })
      .catch((caughtError) => {
        setSandboxError(
          caughtError instanceof Error
            ? caughtError.message
            : "Unknown issue store error"
        );
      });
  }, []);

  async function updateIssueSandbox(path: string) {
    setSandboxLoading(true);
    setSandboxError(null);

    try {
      const response = await fetch(apiURL(path), {
        method: "POST",
      });

      const payload = (await response.json()) as
        | DebugIssueStoreResponse
        | { error?: string };

      if (!response.ok) {
        throw new Error(
          "error" in payload && payload.error
            ? payload.error
            : `Request failed with ${response.status}`
        );
      }

      setIssueCount((payload as DebugIssueStoreResponse).issueCount);
    } catch (caughtError) {
      setSandboxError(
        caughtError instanceof Error
          ? caughtError.message
          : "Unknown issue store error"
      );
    } finally {
      setSandboxLoading(false);
    }
  }

  async function analyze() {
    const trimmed = text.trim();
    if (!trimmed) {
      setError("Issue text is required.");
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(apiURL("/api/v1/debug/issues/analyze"), {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          text: trimmed,
          tags: tags
            .split(",")
            .map((value) => value.trim())
            .filter(Boolean),
        }),
      });

      const payload = await parseAnalyzeResponse(response);

      if (!response.ok) {
        throw new Error(
          "error" in payload && payload.error
            ? payload.error
            : `Request failed with ${response.status}`
        );
      }

      setResult(payload as IssueAnalysis);
    } catch (caughtError) {
      setResult(null);
      setError(
        caughtError instanceof Error
          ? caughtError.message
          : "Unknown analysis error"
      );
    } finally {
      setLoading(false);
    }
  }

  return (
    <AppShell sidebar={<AppSidebar things={SECTION_LINKS} />}>
      <header className="sticky top-0 z-10 shrink-0 border-b bg-background">
        <div className="flex min-h-12 items-center gap-2 px-4">
          <AppShellToggle className="-ml-1" />
          <div className="mr-2 h-4 w-px shrink-0 bg-border" />
          <div>
            <h1 className="text-sm font-medium">Debug analyzer</h1>
            <p className="text-[11px] text-muted-foreground">
              Paste an issue, inspect tags, inspect embedding output.
            </p>
          </div>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-4 py-6 lg:px-6">
          <section
            id="sandbox"
            className="rounded-2xl border border-border/60 bg-card p-5"
          >
            <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Issue sandbox
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  The app starts empty. Load sample issues into the in-memory
                  store here, or clear everything back out.
                </p>
              </div>

              <div className="flex flex-wrap items-center gap-3">
                <span className="rounded-full bg-muted px-3 py-1 text-xs text-muted-foreground">
                  {issueCount == null ? "Checking issues..." : `${issueCount} issues`}
                </span>
                <Button
                  onClick={() => void updateIssueSandbox("/api/v1/debug/issues/sample")}
                  disabled={sandboxLoading}
                >
                  Load sample issues
                </Button>
                <Button
                  variant="outline"
                  onClick={() => void updateIssueSandbox("/api/v1/debug/issues/reset")}
                  disabled={sandboxLoading}
                >
                  Clear issues
                </Button>
              </div>
            </div>

            {sandboxError && (
              <div className="mt-4 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-950">
                {sandboxError}
              </div>
            )}
          </section>

          <section
            id="prompt"
            className="grid gap-6 rounded-2xl border border-border/60 bg-card p-5 lg:grid-cols-[minmax(0,1.4fr)_minmax(20rem,0.8fr)]"
          >
            <div className="space-y-3">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Issue text
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  This page calls the backend debug endpoint directly so you can
                  iterate on model output without touching persistence.
                </p>
              </div>

              <textarea
                value={text}
                onChange={(event) => setText(event.target.value)}
                placeholder="Paste an issue here..."
                className="min-h-64 w-full rounded-xl border border-input bg-background px-3 py-3 text-sm leading-6 outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
              />
            </div>

            <div className="space-y-4">
              <div>
                <label
                  htmlFor="tags"
                  className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground"
                >
                  Tags override
                </label>
                <p className="mt-1 text-sm text-muted-foreground">
                  Optional comma-separated tags. Leave empty to use the backend
                  canonical tag set.
                </p>
              </div>

              <Input
                id="tags"
                value={tags}
                onChange={(event) => setTags(event.target.value)}
                placeholder="bug, safari, export"
              />

              <div className="flex flex-wrap items-center gap-3">
                <Button onClick={analyze} disabled={loading}>
                  {loading ? "Analyzing..." : "Analyze issue"}
                </Button>
                <Button
                  variant="outline"
                  onClick={() => {
                    setText(DEFAULT_TEXT);
                    setTags("");
                  }}
                  disabled={loading}
                >
                  Reset example
                </Button>
              </div>

              {error && (
                <div className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-950">
                  {error}
                </div>
              )}

              {result && (
                <div className="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-950">
                  <p className="font-medium">Analysis complete</p>
                  <p className="mt-1">
                    Tagger: {result.tagger.provider} / {result.tagger.model}
                  </p>
                  <p className="mt-1">
                    Embedder: {result.embedder.provider} /{" "}
                    {result.embedder.model}
                  </p>
                </div>
              )}
            </div>
          </section>

          <section
            id="tags"
            className="rounded-2xl border border-border/60 bg-card p-5"
          >
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Top tags
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Highest relevance scores returned by the backend.
                </p>
              </div>
              {result && (
                <span className="rounded-full bg-muted px-2 py-1 text-xs text-muted-foreground">
                  {result.tags.length} tags
                </span>
              )}
            </div>

            {!result && (
              <p className="mt-4 text-sm text-muted-foreground">
                Run an analysis to inspect tag scores.
              </p>
            )}

            {result && (
              <div className="mt-4 flex flex-wrap gap-2">
                {topTags.map((tag) => (
                  <div
                    key={tag.tag}
                    className="rounded-2xl border border-border bg-muted px-3 py-2 text-sm"
                  >
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{tag.tag}</span>
                      <span className="text-muted-foreground">
                        {formatFloat(tag.relevance)}
                      </span>
                      {tag.suggested && (
                        <span className="rounded-full border border-amber-300 bg-amber-100 px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.14em] text-amber-900">
                          Suggested
                        </span>
                      )}
                    </div>
                    {tag.description && (
                      <p className="mt-1 max-w-72 text-xs leading-5 text-muted-foreground">
                        {tag.description}
                      </p>
                    )}
                  </div>
                ))}
              </div>
            )}
          </section>

          <section
            id="embedding"
            className="rounded-2xl border border-border/60 bg-card p-5"
          >
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Embedding
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Chunking and pooling metadata for the final embedding.
                </p>
              </div>
              {result && (
                <span className="rounded-full bg-muted px-2 py-1 text-xs text-muted-foreground">
                  {result.embedding.dimensions} dims
                </span>
              )}
            </div>

            {!result && (
              <p className="mt-4 text-sm text-muted-foreground">
                Run an analysis to inspect the embedding vector.
              </p>
            )}

            {result && (
              <div className="mt-4 grid gap-3 lg:grid-cols-[minmax(0,16rem)_minmax(0,16rem)_minmax(0,1fr)]">
                <div className="rounded-xl border border-border/70 bg-background px-4 py-3">
                  <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                    Estimated tokens
                  </p>
                  <p className="mt-1 text-lg font-medium">
                    {result.embedding.estimatedTokenCount}
                  </p>
                </div>
                <div className="rounded-xl border border-border/70 bg-background px-4 py-3">
                  <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                    Chunks
                  </p>
                  <p className="mt-1 text-lg font-medium">
                    {result.embedding.chunkCount}
                    {result.embedding.pooledFromChunks ? " pooled" : " direct"}
                  </p>
                </div>
                <div className="rounded-xl border border-border/70 bg-background px-4 py-3">
                  <p className="text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
                    Preview
                  </p>
                  <div className="mt-2 flex flex-wrap gap-2">
                    {embeddingPreview.map((value, index) => (
                      <span
                        key={index}
                        className="rounded-full bg-muted px-2 py-1 font-mono text-xs"
                      >
                        d{index}: {formatFloat(value)}
                      </span>
                    ))}
                  </div>
                </div>
              </div>
            )}
          </section>

          <section
            id="json"
            className="rounded-2xl border border-border/60 bg-card p-5"
          >
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
              Raw JSON
            </p>
            <p className="mt-1 text-sm text-muted-foreground">
              Full response payload from the backend.
            </p>

            <pre className="mt-4 overflow-x-auto rounded-xl border border-border/70 bg-muted/40 p-4 text-xs leading-6 text-foreground/90">
              {result ? JSON.stringify(result, null, 2) : "{\n  \n}"}
            </pre>
          </section>
        </div>
      </div>
    </AppShell>
  );
}
