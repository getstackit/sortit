"use client";

import { useRef, useState } from "react";
import { AppShellToggle } from "@/components/app-shell";

export function SiteHeader({
  issueCount,
  onSubmit,
}: {
  issueCount: number;
  onSubmit: (text: string) => Promise<void> | void;
}) {
  const [input, setInput] = useState("");
  const [focused, setFocused] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const expanded = focused || input.length > 0;

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
      await onSubmit(text);
      setInput("");
      if (textareaRef.current) {
        textareaRef.current.style.height = "auto";
      }
    } catch {
      return;
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <header className="sticky top-0 z-10 shrink-0 border-b bg-background">
      <div className="flex h-12 items-center gap-2 px-4">
        <AppShellToggle className="-ml-1" />
        <div className="mr-2 h-4 w-px shrink-0 bg-border" />
        <h1 className="text-sm font-medium">All issues</h1>
        {issueCount > 0 && (
          <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground tabular-nums">
            {issueCount}
          </span>
        )}

        <div className="relative ml-auto">
          <div
            className={`rounded-md border border-input bg-muted/50 px-3 py-1.5 transition-all duration-200 ${
              expanded
                ? "absolute right-0 top-0 w-[480px] bg-background shadow-md"
                : "w-80"
            }`}
          >
            <textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => {
                setInput(e.target.value);
                autoResize();
              }}
              onFocus={() => setFocused(true)}
              onBlur={() => setFocused(false)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault();
                  void submit();
                }
              }}
              placeholder="paste anything..."
              disabled={submitting}
              className={`w-full resize-none bg-transparent text-[13px] leading-5 placeholder:text-muted-foreground/50 focus:outline-none ${
                expanded ? "max-h-48" : "max-h-5 overflow-hidden"
              }`}
              rows={1}
            />
            {expanded && input.trim() && (
              <div className="flex justify-end border-t border-border/40 pt-1.5 mt-1.5">
                <button
                  onClick={() => void submit()}
                  disabled={submitting}
                  className="shrink-0 rounded-md bg-primary px-2.5 py-0.5 text-[11px] font-medium text-primary-foreground transition-all hover:bg-primary/90 active:scale-[0.97]"
                >
                  {submitting ? "Saving..." : "Submit"}
                </button>
              </div>
            )}
          </div>
        </div>
      </div>
    </header>
  );
}
