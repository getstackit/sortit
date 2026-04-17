import type { EvidenceRange } from "@/lib/issues";

// Backend evidence offsets are UTF-8 byte positions, but JS strings are
// indexed by UTF-16 code units. We precompute a byte->char map once per
// source text so repeated conversions stay cheap.
function buildByteToCharMap(text: string): Uint32Array {
  const encoder = new TextEncoder();
  const byteLen = encoder.encode(text).length;
  const map = new Uint32Array(byteLen + 1);
  let byteIdx = 0;
  for (let i = 0; i < text.length; i++) {
    const code = text.charCodeAt(i);
    let charBytes: number;
    let charUnits = 1;
    if (code < 0x80) {
      charBytes = 1;
    } else if (code < 0x800) {
      charBytes = 2;
    } else if (code >= 0xd800 && code <= 0xdbff) {
      charBytes = 4;
      charUnits = 2;
    } else {
      charBytes = 3;
    }
    for (let b = 0; b < charBytes; b++) {
      map[byteIdx + b] = i;
    }
    byteIdx += charBytes;
    if (charUnits === 2) i += 1;
  }
  map[byteLen] = text.length;
  return map;
}

export function convertEvidenceRanges(
  text: string,
  ranges: EvidenceRange[]
): EvidenceRange[] {
  const map = buildByteToCharMap(text);
  const byteLen = map.length - 1;
  const result: EvidenceRange[] = [];
  for (const range of ranges) {
    const start = Math.max(0, Math.min(byteLen, range.start));
    const end = Math.max(start, Math.min(byteLen, range.end));
    const charStart = map[start];
    const charEnd = map[end];
    if (charEnd > charStart) {
      result.push({ ...range, start: charStart, end: charEnd });
    }
  }
  return mergeRanges(result);
}

export function mergeRanges(ranges: EvidenceRange[]): EvidenceRange[] {
  if (ranges.length <= 1) return ranges.slice();
  const sorted = [...ranges].sort((a, b) => a.start - b.start);
  const merged: EvidenceRange[] = [sorted[0]];
  for (let i = 1; i < sorted.length; i++) {
    const prev = merged[merged.length - 1];
    const next = sorted[i];
    if (next.start <= prev.end) {
      prev.end = Math.max(prev.end, next.end);
    } else {
      merged.push({ ...next });
    }
  }
  return merged;
}
