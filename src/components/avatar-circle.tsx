import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { cn } from "@/lib/utils";

type AvatarCircleProps = {
  name: string;
  size?: "sm" | "md" | "lg";
  className?: string;
};

const SIZE_MAP = {
  sm: "sm",
  md: "default",
  lg: "lg",
} as const;

export function AvatarCircle({ name, size = "md", className }: AvatarCircleProps) {
  return (
    <Avatar size={SIZE_MAP[size]} className={cn("bg-violet-100", className)}>
      <AvatarFallback className="bg-violet-100 font-semibold text-violet-700">
        {name.charAt(0).toUpperCase()}
      </AvatarFallback>
    </Avatar>
  );
}
