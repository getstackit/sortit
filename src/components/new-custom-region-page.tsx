"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { createCustomRegion } from "@/lib/custom-regions";

type TagRow = {
  tag: string;
  minRelevance: string;
};

export function NewCustomRegionPage() {
  const router = useRouter();
  const [label, setLabel] = useState("");
  const [description, setDescription] = useState("");
  const [tagRows, setTagRows] = useState<TagRow[]>([{ tag: "", minRelevance: "0.4" }]);
  const [statuses, setStatuses] = useState<string[]>([]);
  const [maxAgeDays, setMaxAgeDays] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!label.trim()) {
      setError("Label is required");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const tags = tagRows
        .filter((row) => row.tag.trim())
        .map((row) => ({
          tag: row.tag.trim(),
          minRelevance: parseFloat(row.minRelevance) || 0.4,
        }));
      const ageDays = parseInt(maxAgeDays, 10);
      const region = await createCustomRegion({
        label: label.trim(),
        description: description.trim(),
        definition: {
          tags: tags.length > 0 ? tags : undefined,
          statuses: statuses.length > 0 ? statuses : undefined,
          maxAgeDays: Number.isFinite(ageDays) && ageDays > 0 ? ageDays : null,
        },
      });
      router.push(`/regions/custom/${encodeURIComponent(region.id)}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setSubmitting(false);
    }
  };

  const updateTagRow = (index: number, patch: Partial<TagRow>) => {
    setTagRows((rows) => rows.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  };

  const toggleStatus = (status: string) => {
    setStatuses((current) =>
      current.includes(status) ? current.filter((s) => s !== status) : [...current, status],
    );
  };

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        title="New custom region"
        eyebrow="Tier 1"
        subtitle="Define a region from a tag/status/age predicate."
        actions={
          <Link
            href="/regions"
            className="rounded-md border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            Cancel
          </Link>
        }
      />

      <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
        <form onSubmit={handleSubmit} className="flex w-full max-w-2xl flex-col gap-6 px-4 py-6 lg:px-6 xl:px-8">
          <section className="app-surface rounded-[1.5rem] p-5">
            <label className="block text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
              Label
            </label>
            <input
              type="text"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="e.g. p0 auth open"
              className="mt-2 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
              required
            />

            <label className="mt-4 block text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
              Description (optional)
            </label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={2}
              className="mt-2 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
            />
          </section>

          <section className="app-surface rounded-[1.5rem] p-5">
            <h3 className="text-sm font-semibold">Tag predicates (OR)</h3>
            <p className="mt-1 text-xs text-muted-foreground">
              The issue must have at least one of these tags at the given minimum relevance.
            </p>
            <div className="mt-4 space-y-2">
              {tagRows.map((row, index) => (
                <div key={index} className="flex items-center gap-2">
                  <input
                    type="text"
                    value={row.tag}
                    onChange={(e) => updateTagRow(index, { tag: e.target.value })}
                    placeholder="tag name"
                    className="flex-1 rounded-md border border-border bg-background px-3 py-1.5 text-sm"
                  />
                  <input
                    type="number"
                    step="0.05"
                    min="0"
                    max="1"
                    value={row.minRelevance}
                    onChange={(e) => updateTagRow(index, { minRelevance: e.target.value })}
                    className="w-20 rounded-md border border-border bg-background px-3 py-1.5 text-sm tabular-nums"
                  />
                  <button
                    type="button"
                    onClick={() => setTagRows((rows) => rows.filter((_, i) => i !== index))}
                    className="text-xs text-muted-foreground hover:text-foreground"
                  >
                    ×
                  </button>
                </div>
              ))}
              <button
                type="button"
                onClick={() => setTagRows((rows) => [...rows, { tag: "", minRelevance: "0.4" }])}
                className="text-xs text-muted-foreground hover:text-foreground"
              >
                + Add tag
              </button>
            </div>
          </section>

          <section className="app-surface rounded-[1.5rem] p-5">
            <h3 className="text-sm font-semibold">Filters (AND)</h3>

            <div className="mt-4">
              <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                Status
              </p>
              <div className="mt-2 flex gap-2">
                {["open", "closed"].map((status) => (
                  <button
                    key={status}
                    type="button"
                    onClick={() => toggleStatus(status)}
                    className={`rounded-full border px-3 py-1 text-xs font-medium transition-colors ${
                      statuses.includes(status)
                        ? "border-primary bg-primary text-primary-foreground"
                        : "border-border text-muted-foreground hover:bg-muted"
                    }`}
                  >
                    {status}
                  </button>
                ))}
              </div>
            </div>

            <div className="mt-4">
              <label className="block text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                Max age (days, optional)
              </label>
              <input
                type="number"
                value={maxAgeDays}
                onChange={(e) => setMaxAgeDays(e.target.value)}
                min="1"
                placeholder="e.g. 30"
                className="mt-2 w-32 rounded-md border border-border bg-background px-3 py-1.5 text-sm tabular-nums"
              />
            </div>
          </section>

          {error && (
            <div className="app-status-warning rounded-md p-3 text-sm">{error}</div>
          )}

          <div className="flex items-center justify-end gap-2">
            <Link
              href="/regions"
              className="rounded-md border border-border px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              Cancel
            </Link>
            <button
              type="submit"
              disabled={submitting}
              className="rounded-md border border-primary bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground transition-colors hover:opacity-90 disabled:opacity-50"
            >
              {submitting ? "Creating…" : "Create region"}
            </button>
          </div>
        </form>
      </div>
    </AppShell>
  );
}
