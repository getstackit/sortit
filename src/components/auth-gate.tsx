"use client";

import type { ReactNode } from "react";
import { LoginScreen } from "@/components/login-screen";
import { useAuth } from "@/components/auth-provider";

export function AuthGate({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth();

  if (loading) {
    return (
      <div className="flex min-h-svh items-center justify-center bg-background">
        <div className="rounded-full border border-border/70 bg-card px-4 py-2 text-sm text-muted-foreground">
          Loading session...
        </div>
      </div>
    );
  }

  if (!user) {
    return <LoginScreen />;
  }

  return <>{children}</>;
}
