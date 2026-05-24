import { cn } from "@/lib/utils";

type IndicatorProps = {
  className?: string;
  size?: number;
};

export function AnimatedCheckmark({ className, size = 16 }: IndicatorProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      width={size}
      height={size}
      fill="none"
      className={cn("text-emerald-500", className)}
    >
      <path
        d="M3.5 8.5L6.5 11.5L12.5 4.5"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        style={{
          strokeDasharray: 24,
          strokeDashoffset: 0,
          animation: "checkmark-draw 0.4s ease-out",
        }}
      />
    </svg>
  );
}

export function AnimatedX({ className, size = 16 }: IndicatorProps) {
  return (
    <svg
      viewBox="0 0 16 16"
      width={size}
      height={size}
      fill="none"
      className={cn("text-red-500", className)}
      style={{ animation: "shake 0.4s ease-out" }}
    >
      <path
        d="M4.5 4.5L11.5 11.5M11.5 4.5L4.5 11.5"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      />
    </svg>
  );
}

export function PulsingDot({ className, size = 8 }: IndicatorProps) {
  return (
    <span
      className={cn(
        "inline-block rounded-full bg-amber-500",
        className
      )}
      style={{
        width: size,
        height: size,
        animation: "pulse-dot 1.5s ease-in-out infinite",
      }}
    />
  );
}
