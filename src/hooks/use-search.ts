import useSWR from "swr";
import { searchUnified } from "@/lib/search";

export function useUnifiedSearch(query: string, limit = 8) {
  const trimmed = query.trim();
  return useSWR(
    trimmed ? ["unified-search", trimmed, limit] : null,
    () => searchUnified(trimmed, { limit })
  );
}
