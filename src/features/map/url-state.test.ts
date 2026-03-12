import { parseMapURLState, mapQuery } from "@/features/map/url-state";

describe("map url state", () => {
  it("parses batch selection and clamps viewport values", () => {
    const params = new URLSearchParams({
      xMin: "-9",
      xMax: "9",
      yMin: "-2",
      yMax: "4",
      batch: "a, b, a, ,c",
      analyze: "1",
      edgeThreshold: "0.65",
      status: "all",
      sizeTag: "bug",
      issue: "ignored-when-batch-present",
    });

    expect(parseMapURLState(params)).toEqual({
      viewport: {
        xMin: -1.0499999999999998,
        xMax: 1.35,
        yMin: -1.0499999999999998,
        yMax: 1.35,
      },
      selectedId: null,
      batchIds: ["a", "b", "c"],
      showBatchAnalysis: true,
      edgeThreshold: 0.65,
      showClosed: true,
      bubbleSizeTag: "bug",
    });
  });

  it("builds the backend query format used by the map page", () => {
    expect(
      mapQuery(
        { xMin: 0.12345, xMax: 0.98765, yMin: 0.2, yMax: 0.8 },
        0.4,
        false
      )
    ).toBe(
      "xMin=0.1235&xMax=0.9877&yMin=0.2000&yMax=0.8000&edgeThreshold=0.40&status=open"
    );
  });
});
