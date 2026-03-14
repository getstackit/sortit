"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
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
import { createIssue, type IssueRecord } from "@/lib/issues";

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
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) reset();
        onOpenChange(nextOpen);
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Paste anything</DialogTitle>
          <DialogDescription>
            Drop in text, a URL, an error log &mdash; we&apos;ll turn it into an issue,
            and this becomes the first discussion post.
          </DialogDescription>
        </DialogHeader>

        <Textarea
          value={input}
          onChange={(e) => {
            setInput(e.target.value);
            if (error) setError(null);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              void submit();
            }
          }}
          placeholder="paste anything..."
          disabled={submitting}
          className="min-h-[100px] resize-none"
          rows={4}
        />

        {error && (
          <p className="text-xs text-destructive">{error}</p>
        )}

        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>
            Cancel
          </DialogClose>
          <Button
            onClick={() => void submit()}
            disabled={submitting || input.trim().length === 0}
          >
            {submitting ? "Saving..." : "Submit"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
