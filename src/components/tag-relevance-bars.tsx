import { entityStyle } from "@/lib/entity-colors";

export type TagScore = {
  tag: string;
  relevance: number;
};

type TagRelevanceBarsProps = {
  tags: TagScore[];
  colorFor?: (tag: string) => string;
};

function defaultColor(tag: string): string {
  return entityStyle(tag).color;
}

export function TagRelevanceBars({ tags, colorFor = defaultColor }: TagRelevanceBarsProps) {
  if (tags.length === 0) {
    return null;
  }

  return (
    <div className="space-y-2">
      <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
        Tag Relevance
      </p>
      {tags.map(({ tag, relevance }) => (
        <div key={tag} className="flex items-center gap-2">
          <span className="w-20 truncate text-[11px] text-muted-foreground">
            {tag}
          </span>
          <div className="h-1.5 flex-1 rounded-full bg-muted">
            <div
              className="h-full rounded-full"
              style={{
                width: `${relevance * 100}%`,
                backgroundColor: colorFor(tag),
              }}
            />
          </div>
          <span className="w-8 text-right text-[11px] tabular-nums text-muted-foreground">
            {relevance.toFixed(1)}
          </span>
        </div>
      ))}
    </div>
  );
}
