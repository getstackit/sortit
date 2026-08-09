import useSWR from "swr";
import {
  fetchPersonDetail,
  fetchWorkCorrelations,
  type PeopleListStatus,
} from "@/lib/people";

export function usePersonDetail(person: string) {
  const trimmed = person.trim();
  return useSWR(
    trimmed ? ["person-detail", trimmed] : null,
    () => fetchPersonDetail(trimmed)
  );
}

export function useWorkCorrelations(status: PeopleListStatus = "all") {
  return useSWR(
    ["work-correlations", status],
    () => fetchWorkCorrelations(status)
  );
}
