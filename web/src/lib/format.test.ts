import { describe, expect, it } from "vitest";
import { decodeProtoBytes, formatRelativeTime, formatUnixMillis } from "./format";

function toBase64(text: string): string {
  return btoa(text);
}

describe("decodeProtoBytes", () => {
  it("decodes base64 JSON payload", () => {
    const json = toBase64('{"key":"val"}');
    expect(decodeProtoBytes(json)).toBe('{\n  "key": "val"\n}');
  });

  it("decodes base64 plain text", () => {
    expect(decodeProtoBytes(toBase64("hello world"))).toBe("hello world");
  });

  it("returns dash for empty", () => {
    expect(decodeProtoBytes()).toBe("—");
  });
});

describe("formatUnixMillis", () => {
  it("formats numeric milliseconds", () => {
    const ts = Date.UTC(2026, 6, 11, 12, 26, 28);
    expect(formatUnixMillis(ts)).toBe(new Date(ts).toLocaleString());
  });

  it("formats string milliseconds from protojson int64", () => {
    const ts = Date.UTC(2026, 6, 11, 12, 26, 28);
    expect(formatUnixMillis(String(ts))).toBe(new Date(ts).toLocaleString());
  });

  it("returns dash for invalid values", () => {
    expect(formatUnixMillis()).toBe("—");
    expect(formatUnixMillis("")).toBe("—");
    expect(formatUnixMillis(0)).toBe("—");
  });
});

describe("formatRelativeTime", () => {
  it("returns dash for unix epoch zero time", () => {
    expect(formatRelativeTime("1970-01-01T00:00:00Z")).toBe("—");
  });

  it("returns dash for empty input", () => {
    expect(formatRelativeTime()).toBe("—");
  });
});
