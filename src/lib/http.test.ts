import {
  AUTH_UNAUTHORIZED_EVENT,
  HTTPError,
  UnauthorizedError,
  requestJSON,
} from "@/lib/http";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function textResponse(body: string, status: number) {
  return new Response(body, { status });
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("requestJSON", () => {
  it("returns parsed JSON on success", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(jsonResponse({ ok: true }));

    const result = await requestJSON<{ ok: boolean }>("/test");

    expect(result).toEqual({ ok: true });
  });

  it("includes credentials by default", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(jsonResponse({}));

    await requestJSON("/test");

    expect(fetch).toHaveBeenCalledWith(
      "/test",
      expect.objectContaining({ credentials: "include" })
    );
  });

  it("dispatches AUTH_UNAUTHORIZED_EVENT and throws UnauthorizedError on 401", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({ error: "not logged in" }, 401)
    );

    const listener = vi.fn();
    window.addEventListener(AUTH_UNAUTHORIZED_EVENT, listener);

    await expect(requestJSON("/test")).rejects.toThrow(UnauthorizedError);
    expect(listener).toHaveBeenCalledTimes(1);

    window.removeEventListener(AUTH_UNAUTHORIZED_EVENT, listener);
  });

  it("throws HTTPError with status on non-401 failures", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({ error: "not found" }, 404)
    );

    await expect(requestJSON("/test")).rejects.toThrow(HTTPError);

    try {
      await requestJSON("/test");
    } catch (error) {
      expect(error).toBeInstanceOf(HTTPError);
      expect((error as HTTPError).status).toBe(404);
    }
  });

  it("does not dispatch AUTH_UNAUTHORIZED_EVENT on non-401 errors", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({ error: "server error" }, 500)
    );

    const listener = vi.fn();
    window.addEventListener(AUTH_UNAUTHORIZED_EVENT, listener);

    await expect(requestJSON("/test")).rejects.toThrow(HTTPError);
    expect(listener).not.toHaveBeenCalled();

    window.removeEventListener(AUTH_UNAUTHORIZED_EVENT, listener);
  });

  it("reads error message from JSON response body", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({ error: "custom error message" }, 400)
    );

    await expect(requestJSON("/test")).rejects.toThrow("custom error message");
  });

  it("reads error message from plain text response", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      textResponse("plain text error", 400)
    );

    await expect(requestJSON("/test")).rejects.toThrow("plain text error");
  });

  it("falls back to generic message when body is empty", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      textResponse("", 403)
    );

    await expect(requestJSON("/test")).rejects.toThrow(
      "Request failed with 403"
    );
  });
});
