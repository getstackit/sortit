"use client";

import type { ReactNode } from "react";
import { AppShellToggle } from "@/components/app-shell";

type SiteHeaderProps = {
  title: string;
  subtitle?: string;
  eyebrow?: ReactNode;
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
    <header className="sticky top-0 z-10 shrink-0 border-b border-border/70 bg-background/72 backdrop-blur-xl">
      <div className="relative px-4">
        <div className="flex min-h-14 min-w-0 items-center gap-2 py-2">
          <AppShellToggle className="-ml-1" />
          <div className="mr-2 h-4 w-px shrink-0 bg-border" />
          <div className="min-w-0 flex-1">
            {eyebrow && (
              <div className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
                {eyebrow}
              </div>
            )}
            <h1 className="text-sm font-medium">{title}</h1>
          </div>
          {(meta || subtitle) && (
            <div className="ml-auto flex shrink-0 flex-col items-end gap-0.5">
              {meta && <div className="flex items-center gap-2">{meta}</div>}
              {subtitle && (
                <p className="text-[11px] text-muted-foreground">{subtitle}</p>
              )}
            </div>
          )}
        </div>
        {actions && (
          <div className="flex flex-wrap items-center gap-2 pb-3 pl-11">
            {actions}
          </div>
        )}
        <div className="app-gradient-rule animate-gradient-shift absolute inset-x-0 bottom-0 h-px opacity-90" />
      </div>
    </header>
  );
}
