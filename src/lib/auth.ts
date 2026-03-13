import { uiAPIURL } from "@/lib/api";
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

type CompleteCLILoginResponse = CreateAPITokenResponse & {
  status: "complete";
  user: SessionUser;
};

export async function fetchSession(): Promise<SessionUser | null> {
  try {
    const payload = await getJSON<SessionResponse>(uiAPIURL("/auth/session"), {
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
    uiAPIURL("/auth/logout"),
    {}
  );
}

export async function listAPITokens(): Promise<APITokenRecord[]> {
  const payload = await getJSON<APITokensResponse>(uiAPIURL("/auth/tokens"), {
    cache: "no-store",
  });
  return payload.tokens ?? [];
}

export function createAPIToken() {
  return postJSON<CreateAPITokenResponse, Record<string, never>>(
    uiAPIURL("/auth/tokens"),
    {}
  );
}

export function revokeAPIToken(id: string) {
  return postJSON<{ status: string }, Record<string, never>>(
    uiAPIURL(`/auth/tokens/${encodeURIComponent(id)}/revoke`),
    {}
  );
}

export function completeCLILogin(loginID: string) {
  return postJSON<CompleteCLILoginResponse, Record<string, never>>(
    uiAPIURL(`/auth/cli/login/${encodeURIComponent(loginID)}/complete`),
    {}
  );
}
