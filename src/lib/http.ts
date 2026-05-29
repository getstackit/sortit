export const AUTH_UNAUTHORIZED_EVENT = "sortit:unauthorized";

type ErrorPayload = {
  error?: string;
};

export class HTTPError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "HTTPError";
    this.status = status;
  }
}

export class UnauthorizedError extends HTTPError {
  constructor(message: string) {
    super(401, message);
    this.name = "UnauthorizedError";
  }
}

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
    throw new HTTPError(response.status, message);
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

export function putJSON<TResponse, TBody>(
  input: RequestInfo | URL,
  body: TBody,
  init?: RequestInit
) {
  return postJSON<TResponse, TBody>(input, body, { ...init, method: "PUT" });
}

export async function deleteJSON(
  input: RequestInfo | URL,
  init?: RequestInit
): Promise<void> {
  const response = await fetch(input, { ...init, method: "DELETE" });
  if (!response.ok) {
    throw new HTTPError(response.status, await readErrorMessage(response));
  }
}
