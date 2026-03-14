import { cn } from "@/lib/utils";

type AvatarCircleProps = {
  name: string;
  size?: "sm" | "md" | "lg";
  className?: string;
};

const SIZE_CLASSES = {
  sm: "size-7 text-xs",
  md: "size-8 text-sm",
  lg: "size-10 text-base",
};

export function AvatarCircle({ name, size = "md", className }: AvatarCircleProps) {
  return (
    <span
      className={cn(
        "flex items-center justify-center rounded-full bg-violet-100 font-semibold text-violet-700",
        SIZE_CLASSES[size],
        className
      )}
    >
      {name.charAt(0).toUpperCase()}
    </span>
  );
}
