import { cn } from "@/lib/utils"

function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="skeleton"
      className={cn("relative overflow-hidden rounded-md bg-muted/80", className)}
      {...props}
    >
      <div className="animate-shimmer absolute inset-0 bg-gradient-to-r from-transparent via-foreground/8 to-transparent" />
    </div>
  )
}

export { Skeleton }
