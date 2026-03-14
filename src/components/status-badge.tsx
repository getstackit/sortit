import { cn } from "@/lib/utils";

type StatusBadgeVariant = "closed" | "open" | "analyzing" | "failed";

const VARIANT_CLASSES: Record<StatusBadgeVariant, string> = {
  closed: "bg-slate-200 text-slate-700",
  open: "bg-emerald-100 text-emerald-700",
  analyzing: "bg-amber-100 text-amber-700",
  failed: "bg-rose-100 text-rose-700",
};

const VARIANT_LABELS: Record<StatusBadgeVariant, string> = {
  closed: "Closed",
  open: "Open",
  analyzing: "Analyzing",
  failed: "Analysis failed",
};

type StatusBadgeProps = {
  variant: StatusBadgeVariant;
  label?: string;
  compact?: boolean;
  className?: string;
};

export function StatusBadge({ variant, label, compact, className }: StatusBadgeProps) {
  return (
    <span
      className={cn(
        "rounded-full font-medium",
        compact ? "px-1.5 py-px text-[10px]" : "px-2 py-0.5 text-[11px]",
        VARIANT_CLASSES[variant],
        className
      )}
    >
      {label ?? VARIANT_LABELS[variant]}
    </span>
  );
}
