function hashString(str: string): number {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    hash = ((hash << 5) - hash + str.charCodeAt(i)) | 0;
  }
  return Math.abs(hash);
}

export function entityHue(name: string): number {
  return hashString(name) % 360;
}

export function entityColors(name: string) {
  const hue = entityHue(name);

  return {
    bg: `light-dark(hsl(${hue} 30% 95%), hsl(${hue} 40% 22%))`,
    text: `light-dark(hsl(${hue} 55% 35%), hsl(${hue} 60% 80%))`,
    border: `light-dark(hsl(${hue} 40% 85%), hsl(${hue} 45% 35%))`,
    accent: `light-dark(hsl(${hue} 50% 65%), hsl(${hue} 55% 55%))`,
  };
}

export function entityStyle(name: string): React.CSSProperties {
  const colors = entityColors(name);
  return {
    backgroundColor: colors.bg,
    color: colors.text,
    borderColor: colors.border,
  };
}
