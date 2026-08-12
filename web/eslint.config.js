import tseslint from 'typescript-eslint'

/**
 * A deliberately tiny config. It exists for one rule.
 *
 * The workbench is meant to be self-contained, so that deleting the original UI
 * is deleting a directory rather than an archaeology exercise. "Meant to be" is
 * not a property a codebase has — it is one somebody remembers, until they do
 * not. This makes it a build failure instead.
 *
 * Inside the workbench every import is relative, so the directory can be moved
 * or extracted wholesale. Anything absolute is therefore a boundary crossing by
 * construction, and there are only two kinds it is allowed to make:
 *
 *   `@/core/*`  the client SDK and the design tokens — generated types, the API
 *               client, the event stream, formatting, the theme. Not "the old
 *               UI": a second copy would drift from the OpenAPI document it is
 *               generated against.
 *
 *   the one component named below, which has not been ported yet.
 *
 * No recommended ruleset is enabled. The point is one enforceable boundary, not
 * a thousand-warning front across code that already ships.
 */

/**
 * The one component still owned by the original UI: the asset viewer, ~1,000
 * lines of lightbox that is worth keeping rather than rewriting. The artifact
 * gallery left this list when the blueprint table replaced it.
 *
 * Listing the files by name — rather than quietly allowing `@/components/*` —
 * is what keeps one exception from becoming five.
 */
const PENDING_PORT = {
  'src/components/workbench/workbench.tsx': '@/components/asset-viewer',
  'src/components/workbench/documents/video/blueprint-table.tsx': '@/components/asset-viewer',
}

const FORBIDDEN = {
  patterns: [
    {
      group: ['@/components/**', '@/routes/**', '@/core/workspace'],
      message:
        'The workbench owns its UI. Vendor the component into components/workbench/ui, use a relative import if it is already there, or move genuinely shared infrastructure into src/core.',
    },
  ],
}

export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**', 'src/core/schema.d.ts'] },
  {
    files: ['src/components/workbench/**/*.{ts,tsx}'],
    languageOptions: {
      parser: tseslint.parser,
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    rules: { 'no-restricted-imports': ['error', FORBIDDEN] },
  },
  // One narrowed override per un-ported component, naming the single file
  // allowed to reach for it.
  ...Object.entries(PENDING_PORT).map(([file, allowed]) => ({
    files: [file],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              ...FORBIDDEN.patterns[0],
              group: FORBIDDEN.patterns[0].group.filter((pattern) => pattern !== '@/components/**'),
            },
            {
              group: ['@/components/**', `!${allowed}`],
              message: `Only ${allowed} is still allowed here, pending its port into the workbench.`,
            },
          ],
        },
      ],
    },
  })),
)
