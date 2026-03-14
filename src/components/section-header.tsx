import type { ReactNode } from "react";

type SectionHeaderProps = {
  eyebrow: string;
  title: string;
  count?: ReactNode;
  actions?: ReactNode;
};

export function SectionHeader({ eyebrow, title, count, actions }: SectionHeaderProps) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div>
        <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
          {eyebrow}
        </p>
        <h2 className="mt-1 text-lg font-semibold tracking-tight">
          {title}
        </h2>
      </div>
      <div className="flex items-center gap-2">
        {count != null && <span className="app-chip tabular-nums">{count}</span>}
        {actions}
      </div>
    </div>
  );
}
