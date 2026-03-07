"use client";

import { useEffect, useState } from "react";

type BackendHealth = {
  name: string;
  status: string;
  timestamp: string;
  uptime: string;
};

export function BackendStatus() {
  const [data, setData] = useState<BackendHealth | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const response = await fetch("/api/health", {
          cache: "no-store",
        });

        if (!response.ok) {
          throw new Error(`Request failed with ${response.status}`);
        }

        const payload = (await response.json()) as BackendHealth;
        if (!cancelled) {
          setData(payload);
          setError(null);
        }
      } catch (caughtError) {
        if (!cancelled) {
          const message =
            caughtError instanceof Error
              ? caughtError.message
              : "Unknown backend error";
          setError(message);
        }
      }
    }

    load();
    return () => {
      cancelled = true;
    };
  }, []);

  if (error) {
    return (
      <div className="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
        Backend unavailable: {error}
      </div>
    );
  }

  if (!data) {
    return (
      <div className="rounded-xl border border-border/60 bg-card p-4 text-sm text-muted-foreground">
        Waiting for the Go backend...
      </div>
    );
  }

  return (
    <div className="rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-950">
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
