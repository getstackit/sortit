import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

type ScrollAreaProps = ComponentProps<"div"> & {
  orientation?: "vertical" | "horizontal" | "both";
};

export function ScrollArea({
  className,
  orientation = "vertical",
  children,
  ...props
}: ScrollAreaProps) {
  return (
    <div
      className={cn(
        "app-scrollarea",
        orientation === "vertical" && "overflow-y-auto overflow-x-hidden",
        orientation === "horizontal" && "overflow-x-auto overflow-y-hidden",
        orientation === "both" && "overflow-auto",
        className
      )}
      {...props}
    >
      {children}
    </div>
  );
}
