"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Dialog } from "@base-ui/react/dialog";
import { createIssue, type IssueRecord } from "@/lib/issues";
import { cn } from "@/lib/utils";

type PasteModalProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onIssueCreated?: (issue: IssueRecord) => Promise<void> | void;
  navigateOnCreate?: boolean;
};

export function PasteModal({
  open,
  onOpenChange,
  onIssueCreated,
  navigateOnCreate = true,
}: PasteModalProps) {
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

  function reset() {
    setInput("");
    setError(null);
    setSubmitting(false);
  }

  async function submit() {
    const text = input.trim();
    if (!text || submitting) return;

    setSubmitting(true);

    try {
      const created = await createIssue({ raw: text });
      await onIssueCreated?.(created);
      setError(null);
      reset();
      onOpenChange(false);
      if (navigateOnCreate) {
        router.push(`/issues/${created.id}`);
      }
    } catch (caughtError) {
      setError(
        caughtError instanceof Error
          ? caughtError.message
          : "Unknown backend error"
      );
      setSubmitting(false);
    }
  }

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) reset();
        onOpenChange(nextOpen);
      }}
    >
      <Dialog.Portal>
        <Dialog.Backdrop className="fixed inset-0 z-50 bg-black/40 transition-opacity data-[ending-style]:opacity-0 data-[starting-style]:opacity-0" />
        <Dialog.Viewport className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]">
          <Dialog.Popup className="w-full max-w-lg rounded-xl border bg-background p-5 shadow-lg transition-all data-[ending-style]:scale-95 data-[ending-style]:opacity-0 data-[starting-style]:scale-95 data-[starting-style]:opacity-0">
            <Dialog.Title className="text-sm font-medium">
              Paste anything
            </Dialog.Title>
            <Dialog.Description className="mt-1 text-xs text-muted-foreground">
              Drop in text, a URL, an error log — we&apos;ll turn it into an issue.
            </Dialog.Description>

            <textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => {
                setInput(e.target.value);
                if (error) setError(null);
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
              className="mt-4 min-h-[100px] w-full resize-none rounded-md border border-input bg-transparent px-3 py-2 text-sm leading-relaxed placeholder:text-muted-foreground/50 focus:outline-none focus:ring-2 focus:ring-ring/40"
              rows={4}
            />

            {error && (
              <p className="mt-2 text-xs text-destructive">{error}</p>
            )}

            <div className="mt-4 flex items-center justify-end gap-2">
              <Dialog.Close
                className={cn(
                  "rounded-md px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                )}
              >
                Cancel
              </Dialog.Close>
              <button
                onClick={() => void submit()}
                disabled={submitting || input.trim().length === 0}
                className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-all hover:bg-primary/90 active:scale-[0.97] disabled:cursor-not-allowed disabled:opacity-50"
              >
                {submitting ? "Saving..." : "Submit"}
              </button>
            </div>
          </Dialog.Popup>
        </Dialog.Viewport>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
