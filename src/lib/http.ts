export const AUTH_UNAUTHORIZED_EVENT = "splat:unauthorized";

type ErrorPayload = {
  error?: string;
};

export class UnauthorizedError extends Error {}

async function readErrorMessage(response: Response) {
  const fallback = `Request failed with ${response.status}`;
  const contentType = response.headers.get("content-type") ?? "";

  try {
    if (contentType.includes("application/json")) {
      const payload = (await response.json()) as ErrorPayload;
      return payload.error?.trim() || fallback;
    }

    const message = (await response.text()).trim();
    return message || fallback;
  } catch {
    return fallback;
  }
}

export async function requestJSON<T>(
  input: RequestInfo | URL,
  init?: RequestInit
): Promise<T> {
  const response = await fetch(input, {
    credentials: init?.credentials ?? "include",
    ...init,
  });

  if (!response.ok) {
    const message = await readErrorMessage(response);
    if (response.status === 401) {
      if (typeof window !== "undefined") {
        window.dispatchEvent(new CustomEvent(AUTH_UNAUTHORIZED_EVENT));
      }
      throw new UnauthorizedError(message);
    }
    throw new Error(message);
  }

  return (await response.json()) as T;
}

export function getJSON<T>(
  input: RequestInfo | URL,
  init?: RequestInit
) {
  return requestJSON<T>(input, init);
}

export function postJSON<TResponse, TBody>(
  input: RequestInfo | URL,
  body: TBody,
  init?: RequestInit
) {
  const headers = new Headers(init?.headers);
  if (!headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  return requestJSON<TResponse>(input, {
    ...init,
    method: init?.method ?? "POST",
    headers,
    body: JSON.stringify(body),
  });
}
