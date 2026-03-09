"use client";

import { useEffect, useState } from "react";
import { CopyIcon, KeyRoundIcon, RotateCcwIcon } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { useAuth } from "@/components/auth-provider";
import { Button } from "@/components/ui/button";
import {
  createAPIToken,
  listAPITokens,
  revokeAPIToken,
  type APITokenRecord,
} from "@/lib/auth";

export function TokenSettingsPage() {
  const { user } = useAuth();
  const [tokens, setTokens] = useState<APITokenRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createdToken, setCreatedToken] = useState<string | null>(null);

  useEffect(() => {
    void refreshTokens();
  }, []);

  async function refreshTokens() {
    setLoading(true);
    try {
      setTokens(await listAPITokens());
      setError(null);
    } catch (caughtError) {
      setError(caughtError instanceof Error ? caughtError.message : "Failed to load tokens");
    } finally {
      setLoading(false);
    }
  }

  async function handleCreateToken() {
    setCreating(true);
    try {
      const payload = await createAPIToken();
      setCreatedToken(payload.token);
      setTokens((current) => [payload.metadata, ...(current ?? [])]);
      setError(null);
    } catch (caughtError) {
      setError(caughtError instanceof Error ? caughtError.message : "Failed to create token");
    } finally {
      setCreating(false);
    }
  }

  async function handleRevokeToken(id: string) {
    try {
      await revokeAPIToken(id);
      setTokens((current) =>
        (current ?? []).map((token) =>
          token.id === id
            ? { ...token, revokedAt: new Date().toISOString() }
            : token
        )
      );
      setError(null);
    } catch (caughtError) {
      setError(caughtError instanceof Error ? caughtError.message : "Failed to revoke token");
    }
  }

  async function handleCopyToken() {
    if (!createdToken) {
      return;
    }
    await navigator.clipboard.writeText(createdToken);
  }

  const visibleTokens = tokens ?? [];

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        title="API tokens"
        eyebrow="Settings"
        subtitle="Manage the personal bearer token you’ll use for MCP and other non-browser access."
      />

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 px-4 py-6 lg:px-6">
          <section className="app-surface p-5">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Account
                </p>
                <h2 className="mt-2 text-lg font-semibold">{user?.displayName}</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  @{user?.login}
                  {user?.email ? ` • ${user.email}` : ""}
                </p>
              </div>
              <Button onClick={() => void handleCreateToken()} disabled={creating}>
                <KeyRoundIcon className="size-4" />
                {creating ? "Creating..." : "Create token"}
              </Button>
            </div>
          </section>

          {createdToken && (
            <section className="app-surface border-amber-300/60 bg-amber-50/70 p-5 dark:border-amber-500/30 dark:bg-amber-950/20">
              <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                    Copy this now
                  </p>
                  <p className="mt-2 break-all font-mono text-sm text-foreground">{createdToken}</p>
                  <p className="mt-2 text-xs text-muted-foreground">
                    This is the only time the full token will be shown.
                  </p>
                </div>
                <Button variant="outline" onClick={() => void handleCopyToken()}>
                  <CopyIcon className="size-4" />
                  Copy token
                </Button>
              </div>
            </section>
          )}

          <section className="app-surface p-5">
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Active tokens
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Revoke old tokens and mint a new one when you need rotation.
                </p>
              </div>
              <Button variant="outline" onClick={() => void refreshTokens()} disabled={loading}>
                <RotateCcwIcon className="size-4" />
                Refresh
              </Button>
            </div>

            {error && (
              <p className="mt-4 rounded-2xl border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive">
                {error}
              </p>
            )}

            <div className="mt-5 space-y-3">
              {loading ? (
                <p className="text-sm text-muted-foreground">Loading tokens...</p>
              ) : visibleTokens.length === 0 ? (
                <p className="text-sm text-muted-foreground">No tokens yet.</p>
              ) : (
                visibleTokens.map((token) => {
                  const revoked = Boolean(token.revokedAt);
                  return (
                    <div
                      key={token.id}
                      className="flex flex-col gap-3 rounded-2xl border border-border/70 bg-card/60 px-4 py-4 lg:flex-row lg:items-center lg:justify-between"
                    >
                      <div>
                        <p className="font-mono text-sm font-medium">{token.tokenPrefix}...</p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          Created {new Date(token.createdAt).toLocaleString()}
                          {revoked && token.revokedAt
                            ? ` • Revoked ${new Date(token.revokedAt).toLocaleString()}`
                            : ""}
                        </p>
                      </div>
                      <Button
                        variant="outline"
                        disabled={revoked}
                        onClick={() => void handleRevokeToken(token.id)}
                      >
                        {revoked ? "Revoked" : "Revoke"}
                      </Button>
                    </div>
                  );
                })
              )}
            </div>
          </section>
        </div>
      </div>
    </AppShell>
  );
}
