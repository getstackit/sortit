import { entityStyle } from "@/lib/entity-colors";
import { cn } from "@/lib/utils";

type TagBadgeProps = {
  tag: string;
  compact?: boolean;
  className?: string;
};

export function TagBadge({ tag, compact, className }: TagBadgeProps) {
  return (
    <span
      className={cn(
        "rounded-full border font-medium",
        compact ? "px-1.5 py-px text-[10px]" : "px-2 py-0.5 text-[11px]",
        className
      )}
      style={entityStyle(tag)}
    >
      {tag}
    </span>
  );
}
