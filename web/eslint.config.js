import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  // scripts/check-lint-config.mjs and scripts/contrast-report.mjs are both
  // carved OUT of this ignore (see their dedicated blocks below) — leaving
  // either ignored is exactly what let it enumerate in `eslint .`'s file
  // count while having zero rules applied, hiding real bugs (a dead import
  // in one, an unused import in the other).
  globalIgnores(['dist', 'node_modules']),

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
  // parser and lint it with the type-aware TS rule set. projectService
  // resolves real type information from tsconfig.json so no-floating-promises
  // and the no-unsafe-* family actually run — the untyped `recommended` set
  // used previously silently skipped every type-aware rule.
  {
    files: ['**/*.{ts,tsx}'],
    extends: [...tseslint.configs.recommendedTypeChecked, reactHooks.configs.flat.recommended],
    languageOptions: {
      globals: globals.browser,
      parserOptions: {
        ecmaFeatures: { jsx: true },
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
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

      // no-misused-promises' `attributes` check fired 51 times, 50 of them
      // an async handler (or react-hook-form's `form.handleSubmit(cb)`,
      // which always returns a Promise-returning wrapper regardless of
      // whether `cb` is async) passed straight to a DOM/React prop typed
      // `(e) => void` — onClick, onSubmit, onCheckedChange, ErrorState's
      // onRetry. Every one of those 50 was read in full by hand: each
      // wraps its risky `await` in its own try/catch (or, for signOut,
      // calls a function that already swallows its own rejection), so
      // none can actually produce an unhandled rejection — this is
      // TypeScript's own documented void-returning-context allowance for
      // exactly this fire-and-forget event-handler shape, which
      // typescript-eslint's docs name `checksVoidReturn.attributes` for.
      // The 51st (scan-view.tsx passing handleDecode to QrScanner's
      // constructor — an ARGUMENT, not an attribute) and the one genuine
      // bug found in this rule's output (scanner/index.tsx's
      // handleEnterScanMode, fixed separately) are unaffected: only the
      // attributes check is narrowed, `arguments`/`returns` stay on.
      '@typescript-eslint/no-misused-promises': ['error', { checksVoidReturn: { attributes: false } }],
    },
  },

  // scripts/contrast-report.mjs: standalone Node CLI reporting script (the
  // evidence side of the accessibility work — prints measured WCAG contrast
  // read from src/index.css). Read in full: no DOM/browser API, no
  // page.evaluate, nothing but node:fs/node:path and local src/lib imports —
  // Node globals only, unlike pango's screenshot scripts which genuinely mix
  // Node and browser globals via Playwright's page.evaluate.
  {
    files: ['scripts/contrast-report.mjs'],
    extends: [js.configs.recommended],
    languageOptions: {
      globals: globals.node,
      sourceType: 'module',
    },
  },

  // check-lint-config.mjs is ".mjs", matched by no `files` glob above (which
  // covers .{js,jsx,ts,tsx}, never .mjs) — un-ignoring it above alone left it
  // enumerated in `eslint .`'s count with zero rules applied (confirmed via
  // --print-config showing 0 resolved rules, and an injected unused-var
  // probe that went unflagged before this block existed). It runs under
  // Node only.
  {
    files: ['scripts/check-lint-config.mjs'],
    extends: [js.configs.recommended],
    languageOptions: {
      globals: globals.node,
      sourceType: 'module',
    },
  },

  // node:test's `test(name, fn)` / `t.test(name, fn)` registers a test and
  // returns a Promise that the test runner tracks internally — awaiting or
  // voiding it at every call site would not change behaviour, only add
  // noise. This fired 134 times across all 15 test files, every one of them
  // a bare `test(...)`/`t.test(...)` registration call (verified: none was a
  // real await-shaped mistake). Scoped to *.test.ts only — no-floating-promises
  // stays on for every non-test file.
  {
    files: ['**/*.test.ts'],
    rules: {
      '@typescript-eslint/no-floating-promises': 'off',
    },
  },
])
