import { fetchViewportEdges } from "@/features/map/api";
import { HTTPError } from "@/lib/http";

describe("fetchViewportEdges", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("falls back to the full map payload when /map/edges returns a 404 payload", async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ error: "route not found" }), {
          status: 404,
          headers: { "content-type": "application/json" },
        })
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ issues: [], edges: [{ sourceId: "a", targetId: "b", weight: 0.8 }], clusters: [] }), {
          status: 200,
          headers: { "content-type": "application/json" },
        })
      ) as typeof fetch;

    const result = await fetchViewportEdges("xMin=0&xMax=1&yMin=0&yMax=1", new AbortController().signal);

    expect(result).toEqual({
      edges: [{ sourceId: "a", targetId: "b", weight: 0.8 }],
    });
    expect(global.fetch).toHaveBeenCalledTimes(2);
  });

  it("rethrows non-404 HTTP failures", async () => {
    global.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: "upstream failed" }), {
        status: 500,
        headers: { "content-type": "application/json" },
      })
    ) as typeof fetch;

    await expect(
      fetchViewportEdges("xMin=0&xMax=1&yMin=0&yMax=1", new AbortController().signal)
    ).rejects.toEqual(expect.objectContaining<Partial<HTTPError>>({
      status: 500,
      message: "upstream failed",
    }));
  });
});
