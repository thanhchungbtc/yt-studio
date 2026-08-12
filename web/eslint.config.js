import tseslint from 'typescript-eslint'

/**
 * A deliberately tiny config. It exists for one rule.
 *
 * That rule used to guard a boundary between two UIs. There is only one now, so
 * what it guards instead is the shape that made deleting the other one cheap:
 * every import between components is relative, and the only absolute one the UI
 * may make is into `@/core` — the client SDK generated against the server's
 * OpenAPI document, and the design tokens.
 *
 * That is not housekeeping. It is why `components/workbench/**` could be lifted
 * to `components/**` with four one-line edits outside the tree, and it is what
 * keeps the UI a thing you can move rather than a thing you have to unpick.
 */
export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**', 'src/core/schema.d.ts'] },
  {
    files: ['src/components/**/*.{ts,tsx}'],
    languageOptions: {
      parser: tseslint.parser,
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['@/components/**', '@/routes/**'],
              message:
                'Use a relative import between components, or move genuinely shared infrastructure into src/core.',
            },
          ],
        },
      ],
    },
  },
)
