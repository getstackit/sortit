"use client";

import type { ReactNode } from "react";
import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { AppShellToggle } from "@/components/app-shell";
import { createIssue, type IssueRecord } from "@/lib/issues";

type SiteHeaderProps = {
  title: string;
  subtitle?: string;
  eyebrow?: string;
  meta?: ReactNode;
  actions?: ReactNode;
  onIssueCreated?: (issue: IssueRecord) => Promise<void> | void;
  navigateOnCreate?: boolean;
};

export function SiteHeader({
  title,
  subtitle,
  eyebrow,
  meta,
  actions,
  onIssueCreated,
  navigateOnCreate = true,
}: SiteHeaderProps) {
  const router = useRouter();
  const [input, setInput] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  function autoResize() {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight}px`;
  }

  async function submit() {
    const text = input.trim();
    if (!text || submitting) return;

    setSubmitting(true);

    try {
      const created = await createIssue({ raw: text });
      await onIssueCreated?.(created);
      setError(null);
      setInput("");
      if (textareaRef.current) {
        textareaRef.current.style.height = "auto";
      }
      if (navigateOnCreate) {
        router.push(`/issues/${created.id}`);
      }
    } catch (caughtError) {
      setError(
        caughtError instanceof Error
          ? caughtError.message
          : "Unknown backend error"
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <header className="sticky top-0 z-10 shrink-0 border-b bg-background">
      <div className="flex flex-col gap-3 px-4 py-3 xl:flex-row xl:items-start">
        <div className="min-w-0 flex-1">
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

        <div className="w-full xl:ml-auto xl:max-w-[480px]">
          <div className="rounded-md border border-input bg-background px-3 py-2 shadow-sm">
            <textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => {
                setInput(e.target.value);
                if (error) {
                  setError(null);
                }
                autoResize();
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault();
                  void submit();
                }
              }}
              placeholder="paste anything..."
              disabled={submitting}
              className="min-h-12 w-full resize-none bg-transparent text-[13px] leading-5 placeholder:text-muted-foreground/50 focus:outline-none"
              rows={2}
            />
            <div className="mt-2 flex justify-end border-t border-border/40 pt-2">
              <button
                onClick={() => void submit()}
                disabled={submitting || input.trim().length === 0}
                className="shrink-0 rounded-md bg-primary px-2.5 py-1 text-[11px] font-medium text-primary-foreground transition-all hover:bg-primary/90 active:scale-[0.97] disabled:cursor-not-allowed disabled:opacity-50"
              >
                {submitting ? "Saving..." : "Submit"}
              </button>
            </div>
            {error && (
              <p className="mt-2 text-[11px] text-destructive">{error}</p>
            )}
          </div>
        </div>
      </div>
    </header>
  );
}
