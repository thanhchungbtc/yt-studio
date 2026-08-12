import tseslint from 'typescript-eslint'

/**
 * A deliberately tiny config. It exists for one rule.
 *
 * That rule used to guard a boundary between two UIs. There is only one now, so
 * what it guards instead is the shape that made the deletion possible: every
 * import inside `components/workbench` is relative, and the only absolute one it
 * may make is into `@/core` — the client SDK generated against the server's
 * OpenAPI document, and the design tokens.
 *
 * Keeping it means the next thing that gets built here cannot quietly grow a
 * second parallel UI, and that the workbench directory stays movable.
 */
export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**', 'src/core/schema.d.ts'] },
  {
    files: ['src/components/workbench/**/*.{ts,tsx}'],
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
                'Use a relative import inside the workbench, or move genuinely shared infrastructure into src/core.',
            },
          ],
        },
      ],
    },
  },
)
