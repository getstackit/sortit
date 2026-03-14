"use client";

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
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
    <ToggleGroup className={cn("gap-1", className)}>
      {options.map((option) => (
        <ToggleGroupItem
          key={option}
          pressed={value === option}
          onPressedChange={(pressed) => {
            if (pressed) onChange(option);
          }}
          className="rounded-full px-3 py-1 text-xs font-medium data-[pressed]:bg-foreground data-[pressed]:text-background"
          size="sm"
        >
          {formatLabel ? formatLabel(option) : option.charAt(0).toUpperCase() + option.slice(1)}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
