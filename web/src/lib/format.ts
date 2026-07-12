import { HEARTBEAT_STALE_THRESHOLD_SEC, TASK_STATE_META } from "./constants";

function parseTime(iso?: string): number | null {
  if (!iso) return null;
  const t = new Date(iso).getTime();
  if (Number.isNaN(t) || t <= 0) return null;
  return t;
}

export function formatTimestamp(iso?: string): string {
  const t = parseTime(iso);
  if (t === null) return "—";
  return new Date(t).toLocaleString();
}

/** TaskEvent.create_time is Unix milliseconds (proto int64, JSON may be string). */
export function formatUnixMillis(ms?: string | number): string {
  if (ms === undefined || ms === null || ms === "") return "—";
  const n = typeof ms === "string" ? Number(ms) : ms;
  if (!Number.isFinite(n) || n <= 0) return "—";
  const d = new Date(n);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString();
}

export function formatRelativeTime(iso?: string): string {
  const t = parseTime(iso);
  if (t === null) return "—";
  const diff = Date.now() - t;
  const sec = Math.floor(diff / 1000);
  if (sec < 60) return `${sec} 秒前`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min} 分钟前`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr} 小时前`;
  return `${Math.floor(hr / 24)} 天前`;
}

export function decodeProtoBytes(value?: string): string {
  if (!value) return "—";

  let bytes: Uint8Array;
  try {
    const binary = atob(value);
    bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0));
  } catch {
    // Not base64 — show raw (shouldn't happen for proto bytes over JSON)
    return value;
  }

  const text = new TextDecoder("utf-8", { fatal: false }).decode(bytes);
  const trimmed = text.trim();

  if (
    (trimmed.startsWith("{") || trimmed.startsWith("[")) &&
    trimmed.length > 0
  ) {
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2);
    } catch {
      // fall through
    }
  }

  if (text.length > 0 && isMostlyPrintable(text)) {
    return text;
  }

  return toHexPreview(bytes);
}

/** @deprecated use decodeProtoBytes */
export const decodeBytes = decodeProtoBytes;

function isMostlyPrintable(text: string): boolean {
  let printable = 0;
  for (const ch of text) {
    const code = ch.charCodeAt(0);
    if (code === 9 || code === 10 || code === 13 || (code >= 32 && code !== 127)) {
      printable++;
    }
  }
  return printable / text.length >= 0.85;
}

function toHexPreview(bytes: Uint8Array, max = 256): string {
  const slice = bytes.slice(0, max);
  const hex = Array.from(slice, (b) => b.toString(16).padStart(2, "0")).join(
    " ",
  );
  return bytes.length > max ? `${hex} … (${bytes.length} bytes)` : hex;
}

export function formatTaskState(state: string) {
  return (
    TASK_STATE_META[state] ?? {
      label: state,
      variant: "secondary" as const,
    }
  );
}

export function truncateId(id: string, len = 8): string {
  return id.length > len * 2 ? `${id.slice(0, len)}…${id.slice(-4)}` : id;
}

export function isHeartbeatStale(
  iso?: string,
  thresholdSec = HEARTBEAT_STALE_THRESHOLD_SEC,
): boolean {
  const t = parseTime(iso);
  if (t === null) return false;
  return Date.now() - t > thresholdSec * 1000;
}
