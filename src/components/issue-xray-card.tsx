"use client";

import { ScanLineIcon } from "lucide-react";

import { useIssueXRay } from "@/hooks/use-issue-xray";

type Props = {
  issueId: string;
};

const LIFECYCLE_DESCRIPTIONS: Record<string, string> = {
  early: "Recently created, few refinements — meaning may still be moving.",
  mid: "Established — has accumulated context but is still active.",
  late: "Mature — old and refined, likely close to its final shape.",
};

export function IssueXRayCard({ issueId }: Props) {
  const { data, error, isLoading } = useIssueXRay(issueId);

  if (isLoading) {
    return (
      <section className="app-surface rounded-[1.5rem] p-5">
        <Header />
        <p className="mt-4 text-sm text-muted-foreground">Loading X-ray…</p>
      </section>
    );
  }
  if (error) {
    return (
      <section className="app-surface rounded-[1.5rem] p-5">
        <Header />
        <p className="mt-4 text-sm text-muted-foreground">Failed to load X-ray: {error.message}</p>
      </section>
    );
  }
  if (!data) return null;

  return (
    <section className="app-surface rounded-[1.5rem] p-5">
      <Header />

      <ul className="mt-4 space-y-3">
        <ScalarRow
          label="Specificity"
          value={data.specificity}
          tooltip="Relevance-weighted mean specificity of this issue's tags. Higher = sharper subject."
        />
        <ScalarRow
          label="Coherence"
          value={data.coherence}
          tooltip="Mean conditional co-occurrence of this issue's strong tags. Higher = these tags really go together; lower = unusual combination."
        />
        <ScalarRow
          label="Definition"
          value={data.definition}
          tooltip="Dominance of the top tag in the issue's relevance mass. Higher = belongs cleanly to one region."
        />
        <ScalarRow
          label="Connectedness"
          value={data.connectedness.score}
          tooltip={`${data.connectedness.links} link${data.connectedness.links === 1 ? "" : "s"} · ${data.connectedness.operations} operation${data.connectedness.operations === 1 ? "" : "s"}`}
        />
        <li className="flex items-center justify-between gap-3 text-xs">
          <span className="text-muted-foreground">Lifecycle</span>
          <div className="flex items-center gap-2">
            <span
              className="rounded-full border border-border bg-muted/40 px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.14em] text-foreground"
              title={LIFECYCLE_DESCRIPTIONS[data.lifecycle.label]}
            >
              {data.lifecycle.label}
            </span>
            <span className="tabular-nums text-muted-foreground">
              {data.lifecycle.ageDays}d · {data.lifecycle.refinements}{" "}
              refinement{data.lifecycle.refinements === 1 ? "" : "s"}
            </span>
          </div>
        </li>
      </ul>

      <p className="mt-4 text-[11px] text-muted-foreground">
        Tier 2 — a falsifiable scorecard of how this issue sits in the corpus.
      </p>
    </section>
  );
}

function Header() {
  return (
    <div className="flex items-center gap-2">
      <ScanLineIcon className="size-4 text-muted-foreground" />
      <h3 className="text-sm font-semibold">Issue X-ray</h3>
    </div>
  );
}

function ScalarRow({
  label,
  value,
  tooltip,
}: {
  label: string;
  value?: number | null;
  tooltip: string;
}) {
  return (
    <li className="flex items-center gap-3 text-xs" title={tooltip}>
      <span className="w-28 shrink-0 text-muted-foreground">{label}</span>
      <div className="relative h-2 flex-1 rounded-full bg-border">
        {value != null && (
          <div
            className="absolute inset-y-0 left-0 rounded-full bg-primary/60"
            style={{ width: `${Math.max(0, Math.min(1, value)) * 100}%` }}
          />
        )}
      </div>
      <span className="w-10 text-right tabular-nums text-muted-foreground">
        {value != null ? `${Math.round(value * 100)}%` : "—"}
      </span>
    </li>
  );
}
