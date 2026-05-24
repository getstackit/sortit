import useSWR from "swr";
import { searchUnified } from "@/lib/search";

export const UNIFIED_SEARCH_LIMIT = 8;

export function useUnifiedSearch(query: string, limit = UNIFIED_SEARCH_LIMIT) {
  const trimmed = query.trim();
  return useSWR(
    trimmed ? ["unified-search", trimmed, limit] : null,
    () => searchUnified(trimmed, { limit })
  );
}
