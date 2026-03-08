"use client";

import type { ReactNode } from "react";
import { AppShellToggle } from "@/components/app-shell";

type SiteHeaderProps = {
  title: string;
  subtitle?: string;
  eyebrow?: string;
  meta?: ReactNode;
  actions?: ReactNode;
};

export function SiteHeader({
  title,
  subtitle,
  eyebrow,
  meta,
  actions,
}: SiteHeaderProps) {
  return (
    <header className="sticky top-0 z-10 shrink-0 border-b bg-background">
      <div className="px-4 py-3">
        <div className="flex min-w-0 items-start gap-2">
          <AppShellToggle className="-ml-1 mt-0.5" />
          <div className="mr-2 mt-2 h-4 w-px shrink-0 bg-border" />
          <div className="min-w-0">
            {eyebrow && (
              <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
                {eyebrow}
              </p>
            )}
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <h1 className="truncate text-sm font-medium">{title}</h1>
              {meta}
            </div>
            {subtitle && (
              <p className="text-[11px] text-muted-foreground">{subtitle}</p>
            )}
          </div>
        </div>
        {actions && (
          <div className="flex flex-wrap items-center gap-2 pl-11 pt-3">
            {actions}
          </div>
        )}
      </div>
    </header>
  );
}
