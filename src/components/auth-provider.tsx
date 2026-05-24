"use client";

import {
  createContext,
  useContext,
  useEffect,
  useMemo,
} from "react";
import type { ReactNode } from "react";
import useSWR from "swr";
import { AUTH_UNAUTHORIZED_EVENT } from "@/lib/http";
import { fetchSession, logoutSession, type SessionUser } from "@/lib/auth";

type AuthContextValue = {
  user: SessionUser | null;
  loading: boolean;
  refresh: () => Promise<SessionUser | null | undefined>;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const { data, isLoading, mutate } = useSWR("auth-session", fetchSession, {
    revalidateOnFocus: true,
    shouldRetryOnError: false,
  });

  useEffect(() => {
    function handleUnauthorized() {
      void mutate(null, { revalidate: false });
    }

    window.addEventListener(AUTH_UNAUTHORIZED_EVENT, handleUnauthorized);
    return () => window.removeEventListener(AUTH_UNAUTHORIZED_EVENT, handleUnauthorized);
  }, [mutate]);

  const value = useMemo<AuthContextValue>(
    () => ({
      user: data ?? null,
      loading: isLoading,
      refresh: () => mutate(),
      async logout() {
        await logoutSession();
        await mutate(null, { revalidate: false });
      },
    }),
    [data, isLoading, mutate]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return context;
}
