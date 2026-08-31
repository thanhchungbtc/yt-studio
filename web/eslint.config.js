import tseslint from 'typescript-eslint'

/**
 * A deliberately tiny config. It exists to keep one boundary honest.
 *
 * Inside the UI every import is relative. That is not housekeeping: it is why
 * `src/v2/**` can be moved, renamed or lifted with a handful of one-line edits
 * outside the tree, and it is what keeps a UI a thing you can move rather than
 * a thing you have to unpick. Anything genuinely shared belongs in
 * `src/v2/core`, which is reachable relatively from everywhere in the tree.
 *
 * The alias exists for the two files that sit above the UI and mount it. It is
 * not for the UI to reach back out with.
 */
export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**'] },
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
              group: ['@/*'],
              message:
                'The UI is self-contained: import relatively, and put anything shared in src/v2/core.',
            },
          ],
        },
      ],
    },
  },
)
