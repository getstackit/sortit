"use client";

import { useMemo } from "react";
import { marked } from "marked";
import { cn } from "@/lib/utils";

marked.setOptions({
  gfm: true,
  breaks: true,
});

type MarkdownProps = {
  children: string;
  className?: string;
  compact?: boolean;
};

export function Markdown({ children, className, compact }: MarkdownProps) {
  const html = useMemo(() => {
    const result = marked.parse(children);
    if (typeof result === "string") return result;
    return "";
  }, [children]);

  return (
    <div
      className={cn("prose-sortit", compact && "prose-sortit-compact", className)}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
