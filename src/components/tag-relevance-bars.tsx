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
};

function defaultColor(tag: string): string {
  return entityStyle(tag).color ?? "#888";
}

export function TagWeightBar({ tag, relevance, color, generic }: TagWeightBarProps) {
  const barColor = color ?? defaultColor(tag);
  return (
    <div className="flex items-center gap-2">
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
};

export function TagRelevanceBars({ tags, colorFor = defaultColor, tagSpecificity }: TagRelevanceBarsProps) {
  if (tags.length === 0) {
    return null;
  }

  return (
    <div className="space-y-2">
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
          />
        );
      })}
    </div>
  );
}
