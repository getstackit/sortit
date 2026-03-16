"use client";

import { useCallback, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogClose,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

const CLOSE_REASONS = [
  { value: "fixed", label: "Fixed" },
  { value: "stale", label: "Stale" },
  { value: "duplicate", label: "Duplicate" },
  { value: "wont_fix", label: "Won't fix" },
  { value: "by_design", label: "By design" },
] as const;

type CloseIssueModalProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (reason: string, reasonNote: string) => void | Promise<void>;
  issueCount?: number;
};

export function CloseIssueModal({
  open,
  onOpenChange,
  onConfirm,
  issueCount,
}: CloseIssueModalProps) {
  const [reason, setReason] = useState("fixed");
  const [reasonNote, setReasonNote] = useState("");
  const [submitting, setSubmitting] = useState(false);

  function reset() {
    setReason("fixed");
    setReasonNote("");
    setSubmitting(false);
  }

  const submit = useCallback(async () => {
    if (submitting) return;
    setSubmitting(true);
    try {
      await onConfirm(reason, reasonNote.trim());
    } finally {
      setSubmitting(false);
    }
  }, [submitting, reason, reasonNote, onConfirm]);

  const title =
    issueCount && issueCount > 1
      ? `Close ${issueCount} issues`
      : "Close issue";

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) reset();
        onOpenChange(nextOpen);
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            Choose a reason for closing and optionally add a note.
          </DialogDescription>
        </DialogHeader>

        <div className="grid grid-cols-3 gap-2" role="radiogroup" aria-label="Close reason">
          {CLOSE_REASONS.map(({ value, label }) => (
            <button
              key={value}
              type="button"
              role="radio"
              aria-checked={reason === value}
              onClick={() => setReason(value)}
              className={`rounded-md border px-3 py-1.5 text-sm font-medium transition-colors ${
                reason === value
                  ? "border-primary bg-primary text-primary-foreground"
                  : "border-input bg-background text-foreground hover:bg-accent"
              }`}
            >
              {label}
            </button>
          ))}
        </div>

        <Textarea
          value={reasonNote}
          onChange={(e) => setReasonNote(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              void submit();
            }
          }}
          placeholder="Add a note (optional)..."
          disabled={submitting}
          className="min-h-[80px] resize-none"
          rows={3}
        />

        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>
            Cancel
          </DialogClose>
          <Button
            onClick={() => void submit()}
            disabled={submitting}
          >
            {submitting ? "Closing..." : title}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
