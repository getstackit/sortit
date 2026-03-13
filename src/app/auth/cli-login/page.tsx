"use client";

import { Suspense, useEffect, useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import { LoginScreen } from "@/components/login-screen";
import { useAuth } from "@/components/auth-provider";
import { completeCLILogin } from "@/lib/auth";

type CompletionState =
  | { status: "idle" }
  | { status: "done"; tokenPrefix: string; displayName: string }
  | { status: "error"; message: string };

function CLILoginPageFallback() {
  return (
    <div className="flex min-h-svh items-center justify-center bg-background px-4">
      <div className="rounded-full border border-border/70 bg-card px-4 py-2 text-sm text-muted-foreground">
        Loading Splat session...
      </div>
    </div>
  );
}

function CLILoginPageContent() {
  const searchParams = useSearchParams();
  const loginID = searchParams.get("login_id")?.trim() ?? "";
  const { user, loading } = useAuth();
  const [state, setState] = useState<CompletionState>({ status: "idle" });
  const startedLoginID = useRef<string>("");

  useEffect(() => {
    if (!user || !loginID || state.status !== "idle" || startedLoginID.current === loginID) {
      return;
    }

    let cancelled = false;
    startedLoginID.current = loginID;

    void completeCLILogin(loginID)
      .then((response) => {
        if (cancelled) {
          return;
        }
        setState({
          status: "done",
          tokenPrefix: response.metadata.tokenPrefix,
          displayName: response.user.displayName,
        });
      })
      .catch((error: unknown) => {
        if (cancelled) {
          return;
        }
        setState({
          status: "error",
          message: error instanceof Error ? error.message : "Failed to authorize CLI login.",
        });
      });

    return () => {
      cancelled = true;
    };
  }, [loginID, state.status, user]);

  const returnTo = loginID ? `/auth/cli-login?login_id=${encodeURIComponent(loginID)}` : "/auth/cli-login";

  if (!loginID) {
    return (
      <div className="flex min-h-svh items-center justify-center bg-background px-4">
        <div className="rounded-3xl border border-border/60 bg-card px-6 py-5 text-sm text-muted-foreground shadow-sm">
          Missing CLI login identifier.
        </div>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="flex min-h-svh items-center justify-center bg-background px-4">
        <div className="rounded-full border border-border/70 bg-card px-4 py-2 text-sm text-muted-foreground">
          Loading Splat session...
        </div>
      </div>
    );
  }

  if (!user) {
    return <LoginScreen returnTo={returnTo} />;
  }

  return (
    <div className="flex min-h-svh items-center justify-center bg-background px-4 py-12">
      <div className="w-full max-w-lg rounded-[2rem] border border-border/60 bg-card/92 p-8 shadow-[0_24px_80px_rgba(15,23,42,0.10)]">
        <p className="text-[11px] font-medium uppercase tracking-[0.22em] text-muted-foreground">
          Splat CLI
        </p>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight text-foreground">
          {state.status === "done" ? "CLI authorized" : "Authorizing CLI access"}
        </h1>
        <p className="mt-3 text-sm leading-6 text-muted-foreground">
          Signed in as {user.displayName}. This page will mint a personal Splat API token for the CLI.
        </p>

        {state.status === "idle" && (
          <p className="mt-6 text-sm text-muted-foreground">
            Waiting for Splat to issue the access token...
          </p>
        )}

        {state.status === "done" && (
          <>
            <div className="mt-6 rounded-2xl border border-emerald-500/20 bg-emerald-500/8 px-4 py-3 text-sm text-emerald-800">
              The CLI is now authenticated. Token prefix: <span className="font-mono">{state.tokenPrefix}</span>
            </div>
            <p className="mt-4 text-sm text-muted-foreground">
              You can return to the terminal and continue using <span className="font-mono">splat</span>.
            </p>
          </>
        )}

        {state.status === "error" && (
          <div className="mt-6 rounded-2xl border border-destructive/25 bg-destructive/8 px-4 py-3 text-sm text-destructive">
            {state.message}
          </div>
        )}
      </div>
    </div>
  );
}

export default function CLILoginPage() {
  return (
    <Suspense fallback={<CLILoginPageFallback />}>
      <CLILoginPageContent />
    </Suspense>
  );
}
