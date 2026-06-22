"use client";

import { useState, useEffect } from "react";
import {
  CopyIcon,
  KeyRoundIcon,
  RefreshCwIcon,
  RotateCcwIcon,
} from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { useAuth } from "@/components/auth-provider";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { formatDateTime } from "@/lib/format";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
  const [tokenName, setTokenName] = useState("");
  const [revokeTarget, setRevokeTarget] = useState<APITokenRecord | null>(null);
  const [rotateTarget, setRotateTarget] = useState<APITokenRecord | null>(null);

  useEffect(() => {
    void refreshTokens();
  }, []);

  async function refreshTokens() {
    setLoading(true);
    try {
      setTokens(await listAPITokens());
      setError(null);
    } catch (caughtError) {
      setError(
        caughtError instanceof Error
          ? caughtError.message
          : "Failed to load tokens"
      );
    } finally {
      setLoading(false);
    }
  }

  async function handleCreateToken() {
    setCreating(true);
    try {
      const payload = await createAPIToken(tokenName.trim());
      setCreatedToken(payload.token);
      setTokens((current) => [payload.metadata, ...(current ?? [])]);
      setTokenName("");
      setError(null);
    } catch (caughtError) {
      setError(
        caughtError instanceof Error
          ? caughtError.message
          : "Failed to create token"
      );
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
      setError(
        caughtError instanceof Error
          ? caughtError.message
          : "Failed to revoke token"
      );
    }
  }

  async function handleRotateToken(oldToken: APITokenRecord) {
    setCreating(true);
    try {
      const payload = await createAPIToken(oldToken.name);
      await revokeAPIToken(oldToken.id);
      setCreatedToken(payload.token);
      setTokens((current) =>
        [
          payload.metadata,
          ...(current ?? []).map((t) =>
            t.id === oldToken.id
              ? { ...t, revokedAt: new Date().toISOString() }
              : t
          ),
        ]
      );
      setError(null);
    } catch (caughtError) {
      setError(
        caughtError instanceof Error
          ? caughtError.message
          : "Failed to rotate token"
      );
    } finally {
      setCreating(false);
    }
  }

  async function handleCopyToken() {
    if (!createdToken) {
      return;
    }
    await navigator.clipboard.writeText(createdToken);
  }

  function tokenDisplayName(token: APITokenRecord) {
    return token.name || token.tokenPrefix + "...";
  }

  const visibleTokens = tokens ?? [];

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        title="API tokens"
        eyebrow="Settings"
        subtitle="Manage the personal bearer token you'll use for MCP and other non-browser access."
      />

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 px-4 py-6 lg:px-6">
          <section className="app-surface p-5">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
              <div className="flex-1">
                <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
                  Account
                </p>
                <h2 className="mt-2 text-lg font-semibold">
                  {user?.displayName}
                </h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  @{user?.login}
                  {user?.email ? ` • ${user.email}` : ""}
                </p>
                <div className="mt-3">
                  <label className="text-xs font-medium text-muted-foreground">
                    Token name
                  </label>
                  <Input
                    className="mt-1 max-w-xs"
                    placeholder="e.g. MCP server, CI pipeline"
                    value={tokenName}
                    onChange={(e) => setTokenName(e.target.value)}
                  />
                </div>
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
                  <p className="mt-2 break-all font-mono text-sm text-foreground">
                    {createdToken}
                  </p>
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
                  Rotate or revoke tokens as needed.
                </p>
              </div>
              <Button
                variant="outline"
                onClick={() => void refreshTokens()}
                disabled={loading}
              >
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
                <p className="text-sm text-muted-foreground">
                  Loading tokens...
                </p>
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
                        <p className="text-sm font-medium">
                          {token.name ? (
                            <>
                              {token.name}{" "}
                              <span className="font-mono text-muted-foreground">
                                {token.tokenPrefix}...
                              </span>
                            </>
                          ) : (
                            <span className="font-mono">
                              {token.tokenPrefix}...
                            </span>
                          )}
                        </p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          Created{" "}
                          {formatDateTime(token.createdAt)}
                          {token.lastUsedAt
                            ? ` · Last used ${formatDateTime(token.lastUsedAt)}`
                            : " · Never used"}
                          {revoked && token.revokedAt
                            ? ` · Revoked ${formatDateTime(token.revokedAt)}`
                            : ""}
                        </p>
                      </div>
                      <div className="flex gap-2">
                        {!revoked && (
                          <Button
                            variant="outline"
                            onClick={() => setRotateTarget(token)}
                          >
                            <RefreshCwIcon className="size-4" />
                            Rotate
                          </Button>
                        )}
                        <Button
                          variant="outline"
                          disabled={revoked}
                          onClick={() => setRevokeTarget(token)}
                        >
                          {revoked ? "Revoked" : "Revoke"}
                        </Button>
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </section>
        </div>
      </div>

      {/* Revoke confirmation dialog */}
      <Dialog
        open={revokeTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRevokeTarget(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Revoke token</DialogTitle>
            <DialogDescription>
              Are you sure you want to revoke{" "}
              <strong>{revokeTarget ? tokenDisplayName(revokeTarget) : ""}</strong>?
              Any integration using this token will immediately lose access.
              This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setRevokeTarget(null)}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                if (revokeTarget) {
                  void handleRevokeToken(revokeTarget.id);
                  setRevokeTarget(null);
                }
              }}
            >
              Revoke
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Rotate confirmation dialog */}
      <Dialog
        open={rotateTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRotateTarget(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rotate token</DialogTitle>
            <DialogDescription>
              This will create a new token and revoke{" "}
              <strong>{rotateTarget ? tokenDisplayName(rotateTarget) : ""}</strong>.
              You&apos;ll need to update any integration using the old token.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setRotateTarget(null)}
            >
              Cancel
            </Button>
            <Button
              onClick={() => {
                if (rotateTarget) {
                  void handleRotateToken(rotateTarget);
                  setRotateTarget(null);
                }
              }}
            >
              Rotate
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </AppShell>
  );
}
