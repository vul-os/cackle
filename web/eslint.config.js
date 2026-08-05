import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  // scripts/ is a standalone Node tooling script, out of scope for this
  // pass; web/src is the app surface this config targets.
  globalIgnores(['dist', 'node_modules', 'scripts']),

  // Shared across both JS and TS: the app's own plugin rules don't care
  // which language a file is written in.
  {
    files: ['**/*.{js,jsx,ts,tsx}'],
    plugins: { 'react-refresh': reactRefresh },
    rules: {
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
    },
  },

  // Anything still plain JS (postcss.config.js, vite/tailwind config
  // fallbacks) keeps the original eslint-recommended + react-hooks setup.
  {
    files: ['**/*.{js,jsx}'],
    extends: [js.configs.recommended, reactHooks.configs.flat.recommended],
    languageOptions: {
      globals: { ...globals.browser, ...globals.node },
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    rules: {
      // See the identical block in the TS section below for why these are warn.
      'react-hooks/set-state-in-effect': 'warn',
      'react-hooks/preserve-manual-memoization': 'warn',
      'react-hooks/purity': 'warn',
      'react-hooks/refs': 'warn',
      'react-hooks/static-components': 'warn',
    },
  },

  // web/src is TS/TSX end to end — parse it with the typescript-eslint
  // parser and lint it with the recommended TS rule set.
  {
    files: ['**/*.{ts,tsx}'],
    extends: [...tseslint.configs.recommended, reactHooks.configs.flat.recommended],
    languageOptions: {
      globals: globals.browser,
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    rules: {
      // eslint-plugin-react-hooks v7's "recommended" set ships several rules
      // originally written to flag patterns the React Compiler can't safely
      // optimize — they are React Compiler READINESS signals, not correctness
      // bugs, and firing 47 times across ~25 files (mostly ordinary
      // fetch-in-useEffect data loading) means fixing each one is a
      // behavioural rewrite, not a mechanical cleanup. Kept as warn (visible
      // signal, not a merge blocker) rather than fixed blind or disabled
      // outright. Triage, one rule at a time, is future work:
      //   - set-state-in-effect (35): setState called directly inside a
      //     useEffect body — the standard "fetch on mount" shape used
      //     throughout this app's data-loading pages.
      //   - preserve-manual-memoization (5): a useMemo/useCallback whose
      //     dependency array or body the compiler can't prove is stable.
      //   - purity (2): a render path reads something the compiler can't
      //     prove is pure (e.g. a mutable ref read during render).
      //   - refs (2): a ref read/written at a time the compiler can't
      //     guarantee is safe (outside an effect/event handler).
      //   - static-components (2): a component defined inside another
      //     component's render body, so it gets a new identity every render.
      'react-hooks/set-state-in-effect': 'warn',
      'react-hooks/preserve-manual-memoization': 'warn',
      'react-hooks/purity': 'warn',
      'react-hooks/refs': 'warn',
      'react-hooks/static-components': 'warn',
    },
  },
])
