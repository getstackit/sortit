import { Fragment } from "react";
import type { CSSProperties } from "react";
import type { EvidenceRange } from "@/lib/issues";
import { cn } from "@/lib/utils";

export type CitationRange = EvidenceRange & {
  color?: string;
  label?: string;
  active?: boolean;
};

type HighlightedTextProps = {
  text: string;
  ranges: CitationRange[];
  className?: string;
  onCitationHover?: (labels: string[] | null) => void;
};

function uniqueCitations(citations: CitationRange[]) {
  const seen = new Set<string>();
  const out: CitationRange[] = [];
  for (const citation of citations) {
    const key = citation.label ?? `${citation.start}:${citation.end}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(citation);
  }
  return out;
}

function citationLabels(citations: CitationRange[]) {
  return uniqueCitations(citations)
    .map((citation) => citation.label)
    .filter((label): label is string => Boolean(label));
}

function formatCitationKind(kind?: string) {
  switch (kind) {
    case "direct_quote":
      return "Direct quote";
    default:
      return "Cited evidence";
  }
}

function formatCitationSource(source?: string) {
  switch (source) {
    case "source_text":
      return "Canonical summary";
    default:
      return "Canonical summary";
  }
}

function stackedUnderlineStyle(citations: CitationRange[]): CSSProperties {
  const colors = uniqueCitations(citations)
    .map((citation) => citation.color)
    .filter((color): color is string => Boolean(color))
    .slice(0, 3);

  if (colors.length <= 1) {
    return {
      textDecorationColor: colors[0],
    };
  }

  return {
    textDecorationColor: colors[0],
    backgroundImage: colors.map((color) => `linear-gradient(${color}, ${color})`).join(", "),
    backgroundPosition: colors
      .map((_, index) => `0 calc(100% - ${index * 3}px)`)
      .join(", "),
    backgroundRepeat: "no-repeat",
    backgroundSize: colors.map(() => "100% 2px").join(", "),
    paddingBottom: `${(colors.length - 1) * 3}px`,
  };
}

export function HighlightedText({
  text,
  ranges,
  className,
  onCitationHover,
}: HighlightedTextProps) {
  const normalized = ranges
    .map((range) => ({
      ...range,
      start: Math.max(0, Math.min(text.length, range.start)),
      end: Math.max(0, Math.min(text.length, range.end)),
    }))
    .filter((range) => range.end > range.start);
  const breakpoints = new Set<number>([0, text.length]);
  for (const range of normalized) {
    breakpoints.add(range.start);
    breakpoints.add(range.end);
  }

  const sortedBreakpoints = [...breakpoints].sort((a, b) => a - b);
  const segments: Array<{
    text: string;
    citations: CitationRange[];
  }> = [];
  let cursor = 0;

  for (let i = 1; i < sortedBreakpoints.length; i++) {
    const start = sortedBreakpoints[i - 1];
    const end = sortedBreakpoints[i];
    if (start > cursor) {
      segments.push({ text: text.slice(cursor, start), citations: [] });
    }
    const citations = normalized.filter(
      (range) => range.start < end && range.end > start
    );
    segments.push({ text: text.slice(start, end), citations });
    cursor = end;
  }

  if (cursor < text.length) {
    segments.push({ text: text.slice(cursor), citations: [] });
  }

  return (
    <div className={cn("whitespace-pre-wrap text-sm leading-relaxed", className)}>
      {segments.map((segment, i) => {
        const activeCitation = segment.citations.find((citation) => citation.active);
        const visibleCitation = activeCitation ?? segment.citations[0];
        const labels = citationLabels(segment.citations);
        const citedBy = labels.join(", ");
        const source = formatCitationSource(visibleCitation?.source);
        const kind = formatCitationKind(visibleCitation?.kind);

        return (
          <Fragment key={i}>
            {visibleCitation ? (
              <mark
                tabIndex={0}
                aria-label={
                  citedBy
                    ? `${kind} for ${citedBy} from ${source}`
                    : `${kind} from ${source}`
                }
                onMouseEnter={() => onCitationHover?.(labels)}
                onMouseLeave={() => onCitationHover?.(null)}
                onFocus={() => onCitationHover?.(labels)}
                onBlur={() => onCitationHover?.(null)}
                className={cn(
                  "group/citation relative rounded px-0.5 text-foreground underline decoration-2 underline-offset-2 focus:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                  activeCitation
                    ? "bg-amber-200/70 dark:bg-amber-400/30"
                    : "bg-transparent underline decoration-dotted"
                )}
                style={stackedUnderlineStyle(segment.citations)}
                title={citedBy ? `Cited by ${citedBy}` : "Cited evidence"}
              >
                {segment.text}
                <span className="pointer-events-none absolute left-0 top-full z-20 mt-1 hidden min-w-max max-w-xs rounded-md border border-border bg-popover px-2 py-1 text-[11px] font-normal leading-4 text-popover-foreground shadow-md group-hover/citation:block group-focus/citation:block">
                  <span className="block font-medium">
                    {citedBy ? `Cited by ${citedBy}` : "Cited evidence"}
                  </span>
                  <span className="block text-muted-foreground">
                    {kind} · {source}
                  </span>
                </span>
              </mark>
            ) : (
              segment.text
            )}
          </Fragment>
        );
      })}
    </div>
  );
}
