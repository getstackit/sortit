const RELATIVE_TIME_RANGES: Array<[Intl.RelativeTimeFormatUnit, number]> = [
  ["year", 60 * 60 * 24 * 365],
  ["month", 60 * 60 * 24 * 30],
  ["week", 60 * 60 * 24 * 7],
  ["day", 60 * 60 * 24],
  ["hour", 60 * 60],
  ["minute", 60],
];

export function formatRelativeTime(value: string) {
  const timestamp = new Date(value).getTime();
  const diffSeconds = Math.round((timestamp - Date.now()) / 1000);
  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });

  for (const [unit, seconds] of RELATIVE_TIME_RANGES) {
    if (Math.abs(diffSeconds) >= seconds || unit === "minute") {
      return formatter.format(Math.round(diffSeconds / seconds), unit);
    }
  }

  return "just now";
}

export function scoreClasses(score: number) {
  if (score >= 0.65) return "bg-emerald-100 text-emerald-700";
  if (score >= 0.35) return "bg-amber-100 text-amber-700";
  return "bg-slate-100 text-slate-700";
}

export function scoreColor(score: number) {
  if (score >= 0.7) return "text-emerald-700 bg-emerald-100";
  if (score >= 0.4) return "text-amber-700 bg-amber-100";
  return "text-slate-600 bg-slate-100";
}
