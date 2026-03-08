import useSWR from "swr";
import { apiURL } from "@/lib/api";
import { getJSON } from "@/lib/http";

type BackendHealth = {
  name: string;
  status: string;
  timestamp: string;
  uptime: string;
};

export function useBackendHealth() {
  return useSWR("backend-health", () =>
    getJSON<BackendHealth>(apiURL("/api/health"), { cache: "no-store" })
  );
}
