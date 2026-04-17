import Link from "next/link";
import { entityStyle } from "@/lib/entity-colors";
import { tagHref } from "@/lib/tags";

export type TagScore = {
  tag: string;
  relevance: number;
};

type TagWeightBarProps = {
  tag: string;
  relevance: number;
  color?: string;
  generic?: boolean;
  hasEvidence?: boolean;
  evidenceCount?: number;
  active?: boolean;
  onHover?: (tag: string | null) => void;
};

export function defaultTagColor(tag: string): string {
  return entityStyle(tag).color ?? "#888";
}

export function TagWeightBar({
  tag,
  relevance,
  color,
  generic,
  hasEvidence,
  evidenceCount,
  active,
  onHover,
}: TagWeightBarProps) {
  const barColor = color ?? defaultTagColor(tag);
  const interactive = Boolean(onHover) && hasEvidence !== false;
  const citationCount = evidenceCount ?? (hasEvidence ? 1 : 0);
  return (
    <div
      className={`flex items-center gap-2 rounded-md px-1 py-0.5 transition-colors ${
        interactive ? "cursor-pointer" : ""
      } ${active ? "bg-amber-200/40 dark:bg-amber-400/15" : ""}`}
      onMouseEnter={interactive ? () => onHover?.(tag) : undefined}
      onMouseLeave={interactive ? () => onHover?.(null) : undefined}
    >
      <Link
        href={tagHref(tag)}
        className={`w-20 truncate text-[11px] hover:underline ${generic ? "text-muted-foreground/50" : "text-muted-foreground"}`}
      >
        {tag}
      </Link>
      <div className="h-1.5 flex-1 rounded-full bg-muted">
        <div
          className="h-full rounded-full"
          style={{
            width: `${relevance * 100}%`,
            backgroundColor: barColor,
            opacity: generic ? 0.5 : 1,
          }}
        />
      </div>
      <span className="w-8 text-right text-[11px] tabular-nums text-muted-foreground">
        {relevance.toFixed(1)}
      </span>
      {citationCount > 0 && (
        <span
          className="text-[9px] text-muted-foreground/70"
          title={`${citationCount} source-text citation${citationCount === 1 ? "" : "s"}`}
        >
          {citationCount} cite{citationCount === 1 ? "" : "s"}
        </span>
      )}
      {hasEvidence === false && citationCount === 0 && (
        <span
          className="text-[9px] text-muted-foreground/60"
          title="No source-text evidence was cited for this tag"
        >
          no cite
        </span>
      )}
      {generic && (
        <Link
          href={tagHref(tag)}
          className="text-[9px] text-muted-foreground/60 hover:text-foreground"
          title="Broad bucket tag — see more specific alternatives on the tag map"
        >
          broad
        </Link>
      )}
    </div>
  );
}

type TagRelevanceBarsProps = {
  tags: TagScore[];
  colorFor?: (tag: string) => string;
  tagSpecificity?: Map<string, number | null | undefined>;
  hasEvidence?: (tag: string) => boolean;
  evidenceCount?: (tag: string) => number;
  activeTag?: string | null;
  activeTags?: string[];
  onHover?: (tag: string | null) => void;
};

export function TagRelevanceBars({
  tags,
  colorFor = defaultTagColor,
  tagSpecificity,
  hasEvidence,
  evidenceCount,
  activeTag,
  activeTags,
  onHover,
}: TagRelevanceBarsProps) {
  if (tags.length === 0) {
    return null;
  }

  return (
    <div className="space-y-1">
      {tags.map(({ tag, relevance }) => {
        const specificity = tagSpecificity?.get(tag) ?? null;
        const generic = specificity !== null && specificity < 0.3;
        return (
          <TagWeightBar
            key={tag}
            tag={tag}
            relevance={relevance}
            color={colorFor(tag)}
            generic={generic}
            hasEvidence={hasEvidence?.(tag)}
            evidenceCount={evidenceCount?.(tag)}
            active={activeTag === tag || Boolean(activeTags?.includes(tag))}
            onHover={onHover}
          />
        );
      })}
    </div>
  );
}
