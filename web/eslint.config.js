import tseslint from 'typescript-eslint'

/**
 * A deliberately tiny config. It exists to keep two boundaries honest.
 *
 * Inside a UI, every import between components is relative, and the only
 * absolute one it may make is into its own root. That is not housekeeping: it
 * is why `components/workbench/**` could be lifted to `components/**` with four
 * one-line edits outside the tree, and it is what keeps a UI a thing you can
 * move rather than a thing you have to unpick.
 *
 * Between the two UIs, nothing. `src/v2` is self-contained by rule — it carries
 * its own client, its own tokens and its own components — so that v1 can be
 * deleted in one commit rather than unpicked in twenty. The rule is what makes
 * that promise checkable instead of aspirational.
 */
export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**', 'src/core/schema.d.ts'] },
  {
    files: ['src/v2/**/*.{ts,tsx}'],
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
              group: ['@/*', '!@/v2/**'],
              message:
                'V2 is self-contained: import relatively within src/v2, and copy in what you need rather than reaching into v1.',
            },
          ],
        },
      ],
    },
  },
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
