import useSWR from "swr";
import {
  fetchPersonProfile,
  fetchWorkCorrelations,
  type PeopleListStatus,
} from "@/lib/people";

export function usePersonProfile(person: string, status: PeopleListStatus = "all") {
  const trimmed = person.trim();
  return useSWR(
    trimmed ? ["person-profile", trimmed, status] : null,
    () => fetchPersonProfile(trimmed, status)
  );
}

export function useWorkCorrelations(status: PeopleListStatus = "all") {
  return useSWR(
    ["work-correlations", status],
    () => fetchWorkCorrelations(status)
  );
}
