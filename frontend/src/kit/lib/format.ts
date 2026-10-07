const compact = new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 });
const relative = new Intl.RelativeTimeFormat("en", { numeric: "auto", style: "short" });
const shortDate = new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" });
const monthYear = new Intl.DateTimeFormat(undefined, { month: "long", year: "numeric" });

/** 12345 → "12.3K" */
export function formatTokens(n: number): string {
  if (!n) return "0";
  return n < 1000 ? String(n) : compact.format(n);
}

/** 0.0321 → "$0.03" */
export function formatCost(usd: number): string {
  if (!usd) return "$0.00";
  if (usd < 0.01) return "<$0.01";
  return `$${usd.toFixed(2)}`;
}

/** 83_000 ms → "1m 23s" */
export function formatDuration(ms: number): string {
  if (!ms || ms < 0) return "0s";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}

/** Unix ms → "3 min. ago" / "yesterday" */
export function formatRelative(ms: number, now = Date.now()): string {
  const diff = (ms - now) / 1000;
  const abs = Math.abs(diff);
  if (abs < 45) return "just now";
  if (abs < 3600) return relative.format(Math.round(diff / 60), "minute");
  if (abs < 86400) return relative.format(Math.round(diff / 3600), "hour");
  if (abs < 86400 * 7) return relative.format(Math.round(diff / 86400), "day");
  return shortDate.format(ms);
}

/** Group label for a timestamp in session lists. */
export function dayGroup(ms: number, now = new Date()): string {
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const day = 86400000;
  if (ms >= startOfToday) return "Today";
  if (ms >= startOfToday - day) return "Yesterday";
  if (ms >= startOfToday - 6 * day) return "Previous 7 days";
  if (ms >= startOfToday - 29 * day) return "Previous 30 days";
  return monthYear.format(ms);
}

/** Absolute path → path relative to root when inside it. */
export function relativePath(path: string, root?: string): string {
  if (!path) return "";
  if (root && path.startsWith(root.endsWith("/") ? root : root + "/")) {
    return path.slice(root.length + (root.endsWith("/") ? 0 : 1));
  }
  const home = homeDir();
  if (home && path.startsWith(home + "/")) return "~" + path.slice(home.length);
  return path;
}

/** Last path segment. */
export function basename(path: string): string {
  const trimmed = path.replace(/\/+$/, "");
  const i = trimmed.lastIndexOf("/");
  return i >= 0 ? trimmed.slice(i + 1) : trimmed;
}

/** Directory part of a path. */
export function dirname(path: string): string {
  const i = path.replace(/\/+$/, "").lastIndexOf("/");
  return i > 0 ? path.slice(0, i) : "";
}

let cachedHome: string | undefined;
/** The user's home directory, once known (see setHomeDir). */
export function homeDir(): string | undefined {
  return cachedHome;
}
/** Records the user's home directory for "~" abbreviation. */
export function setHomeDir(dir: string) {
  cachedHome = dir.replace(/\/+$/, "");
}

/** Count lines in a string ("" → 0). */
export function lineCount(s: string | undefined): number {
  if (!s) return 0;
  let n = 1;
  for (let i = 0; i < s.length; i++) if (s.charCodeAt(i) === 10) n++;
  if (s.endsWith("\n")) n--;
  return n;
}

/** Truncate to n chars with an ellipsis. */
export function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + "…" : s;
}

/** Human-readable byte size. */
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

const natural = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });
/** Finder-like name order: case-insensitive, numbers by value ("file2" before "file10"). */
export const compareNames = natural.compare;
