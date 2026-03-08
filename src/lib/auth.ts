import { apiURL } from "@/lib/api";
import { getJSON, postJSON, UnauthorizedError } from "@/lib/http";

export type SessionUser = {
  id: string;
  login: string;
  displayName: string;
  avatarUrl?: string;
  email?: string;
};

export type SessionResponse = {
  user: SessionUser;
};

export type APITokenRecord = {
  id: string;
  tokenPrefix: string;
  createdAt: string;
  revokedAt?: string | null;
};

type APITokensResponse = {
  tokens: APITokenRecord[];
};

type CreateAPITokenResponse = {
  token: string;
  metadata: APITokenRecord;
};

export async function fetchSession(): Promise<SessionUser | null> {
  try {
    const payload = await getJSON<SessionResponse>(apiURL("/api/v1/auth/session"), {
      cache: "no-store",
    });
    return payload.user;
  } catch (error) {
    if (error instanceof UnauthorizedError) {
      return null;
    }
    throw error;
  }
}

export function logoutSession() {
  return postJSON<{ status: string }, Record<string, never>>(
    apiURL("/api/v1/auth/logout"),
    {}
  );
}

export async function listAPITokens(): Promise<APITokenRecord[]> {
  const payload = await getJSON<APITokensResponse>(apiURL("/api/v1/auth/tokens"), {
    cache: "no-store",
  });
  return payload.tokens;
}

export function createAPIToken() {
  return postJSON<CreateAPITokenResponse, Record<string, never>>(
    apiURL("/api/v1/auth/tokens"),
    {}
  );
}

export function revokeAPIToken(id: string) {
  return postJSON<{ status: string }, Record<string, never>>(
    apiURL(`/api/v1/auth/tokens/${encodeURIComponent(id)}/revoke`),
    {}
  );
}
