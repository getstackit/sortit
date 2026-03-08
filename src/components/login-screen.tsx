"use client";

import { LogInIcon } from "lucide-react";
import { apiURL } from "@/lib/api";

export function LoginScreen() {
  return (
    <div className="relative isolate flex min-h-svh items-center justify-center overflow-hidden px-4 py-12">
      <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,color-mix(in_oklab,var(--gradient-start)_20%,transparent)_0%,transparent_36%),radial-gradient(circle_at_bottom_right,color-mix(in_oklab,var(--gradient-end)_18%,transparent)_0%,transparent_34%),linear-gradient(180deg,color-mix(in_oklab,var(--background)_92%,white_8%)_0%,var(--background)_100%)]" />
      <div className="relative w-full max-w-md overflow-hidden rounded-[2rem] border border-border/60 bg-card/88 p-8 shadow-[0_24px_80px_rgba(15,23,42,0.10)] backdrop-blur-xl">
        <div className="inline-flex size-12 items-center justify-center rounded-2xl text-xl font-semibold text-white shadow-sm"
          style={{
            background:
              "linear-gradient(135deg, var(--gradient-start), var(--gradient-mid) 55%, var(--gradient-end))",
          }}
        >
          s
        </div>
        <p className="mt-6 text-[11px] font-medium uppercase tracking-[0.22em] text-muted-foreground">
          Splat
        </p>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight text-foreground">
          Sign in with GitHub
        </h1>
        <p className="mt-3 text-sm leading-6 text-muted-foreground">
          GitHub is the only sign-in method right now. The backend owns auth, and your session unlocks the web UI plus personal API tokens for MCP access.
        </p>

        <a
          href={apiURL("/api/v1/auth/github/start")}
          className="mt-8 inline-flex w-full items-center justify-center gap-2 rounded-2xl border border-border/60 bg-foreground px-4 py-3 text-sm font-medium text-background transition-transform hover:-translate-y-0.5"
        >
          <LogInIcon className="size-4" />
          Continue with GitHub
        </a>

        <p className="mt-4 text-center text-xs text-muted-foreground">
          After sign-in, you’ll return here automatically.
        </p>
      </div>
    </div>
  );
}
