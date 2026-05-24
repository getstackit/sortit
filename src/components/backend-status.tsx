"use client";

import { useBackendHealth } from "@/hooks/use-backend-health";

export function BackendStatus() {
  const { data, error } = useBackendHealth();

  if (error) {
    return (
      <div className="app-status-warning">
        Backend unavailable: {error.message}
      </div>
    );
  }

  if (!data) {
    return (
      <div className="app-subtle-surface p-4 text-sm text-muted-foreground">
        Waiting for the Go backend...
      </div>
    );
  }

  return (
    <div className="app-status-success">
      <div className="flex items-center justify-between gap-3">
        <span className="font-medium">Backend connected</span>
        <span className="rounded-full bg-emerald-200 px-2 py-1 text-[10px] uppercase tracking-[0.18em]">
          {data.status}
        </span>
      </div>
      <p className="mt-3">{data.name}</p>
      <p className="mt-1">Uptime: {data.uptime}</p>
      <p className="mt-1">
        Updated: {new Date(data.timestamp).toLocaleTimeString()}
      </p>
    </div>
  );
}
