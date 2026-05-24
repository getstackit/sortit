import useSWR from "swr";
import { fetchTags } from "@/lib/tags";

export function useTags() {
  return useSWR("tags", () => fetchTags());
}
