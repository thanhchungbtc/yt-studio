import type { CSSProperties } from "react";

/**
 * The curated colors for projects and labels, in the order they are handed
 * out (gray only by hand). Stored by name; each has a light and a dark shade
 * (--c-<name>) and a readable text shade (--c-<name>-fg) in globals.css.
 * Mirrors domain.Palette.
 */
export const PALETTE = [
  { key: "blue", name: "Blue" },
  { key: "green", name: "Green" },
  { key: "orange", name: "Orange" },
  { key: "purple", name: "Purple" },
  { key: "pink", name: "Pink" },
  { key: "teal", name: "Teal" },
  { key: "red", name: "Red" },
  { key: "amber", name: "Amber" },
  { key: "indigo", name: "Indigo" },
  { key: "gray", name: "Gray" },
] as const;

export type PaletteColor = (typeof PALETTE)[number]["key"];

const keys = new Set<string>(PALETTE.map((c) => c.key));

/** A palette color, falling back to gray for unknown values. */
export function paletteColor(color: string | undefined): PaletteColor {
  return color && keys.has(color) ? (color as PaletteColor) : "gray";
}

/** CSS color of a palette color's swatch / dot. */
export function colorVar(color: string | undefined): string {
  return `var(--c-${paletteColor(color)})`;
}

/** Style setting --lc (swatch) and --lcf (text) for tinted pills. */
export function colorStyle(color: string | undefined): CSSProperties {
  const c = paletteColor(color);
  return { "--lc": `var(--c-${c})`, "--lcf": `var(--c-${c}-fg)` } as CSSProperties;
}

/** "Needs review" → "needs-review", the form used in `label:` searches. */
export function labelSlug(name: string): string {
  return name.trim().toLowerCase().replace(/\s+/g, "-");
}
