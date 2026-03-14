import { cn } from "@/lib/utils";

type FilterToggleGroupProps<T extends string> = {
  options: readonly T[];
  value: T;
  onChange: (value: T) => void;
  formatLabel?: (value: T) => string;
  className?: string;
};

export function FilterToggleGroup<T extends string>({
  options,
  value,
  onChange,
  formatLabel,
  className,
}: FilterToggleGroupProps<T>) {
  return (
    <div className={cn("flex items-center gap-2", className)}>
      {options.map((option) => (
        <button
          key={option}
          type="button"
          onClick={() => onChange(option)}
          className={cn(
            "rounded-full px-3 py-1 text-xs font-medium transition-colors",
            value === option
              ? "bg-foreground text-background"
              : "bg-muted text-muted-foreground hover:bg-muted/80"
          )}
        >
          {formatLabel ? formatLabel(option) : option.charAt(0).toUpperCase() + option.slice(1)}
        </button>
      ))}
    </div>
  );
}
