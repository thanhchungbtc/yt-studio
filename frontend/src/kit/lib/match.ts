/**
 * Ranks how well `value` matches a search query, for the command palette.
 * Returns 0 (hidden) … 1 (best). Deliberately stricter than letter-by-letter
 * fuzzy matching, which surfaces unrelated items ("new" → "Next Tab").
 *
 *   1.0  value starts with the query
 *   0.9  query appears as a substring
 *   0.7  every query word starts a word in value ("tog pan" → Toggle Panel)
 *   0.5  query matches word initials ("tbp" → Toggle Bottom Panel)
 *
 * `label:name` in the query keeps only items carrying that label.
 */
export function matchScore(value: string, search: string): number {
  // `label:name` tokens filter: the value must carry each label (as a
  // "label:<slug>" token, prefix allowed); the rest of the query ranks.
  const labels = Array.from(search.matchAll(/(?:^|\s)label:(\S+)/gi), (m) => m[1].toLowerCase());
  if (labels.length > 0) {
    const v = value.toLowerCase();
    if (!labels.every((l) => v.includes(`label:${l}`))) return 0;
    const rest = search.replace(/(?:^|\s)label:\S+/gi, " ").trim();
    return rest ? matchScore(value, rest) : 1;
  }
  const q = search.toLowerCase().trim();
  if (!q) return 1;
  const v = value.toLowerCase();
  if (v.startsWith(q)) return 1;
  if (v.includes(q)) return 0.9;
  const words = v.split(/[\s:./_\-()]+/).filter(Boolean);
  const terms = q.split(/\s+/);
  if (terms.every((t) => words.some((w) => w.startsWith(t)))) return 0.7;
  const initials = words.map((w) => w[0]).join("");
  if (q.length >= 2 && initials.includes(q.replace(/\s+/g, ""))) return 0.5;
  return 0;
}
