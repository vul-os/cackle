# docs/screenshots

**Screenshots were not generated.**

`scripts/screenshots.mjs` could not get Cackle running in demo mode: Command failed: npm run build

/api/placeholder/8/8 referenced in /api/placeholder/8/8 didn't resolve at build time, it will remain unchanged to be resolved at runtime
x Build failed in 5.99s
error during build:
Could not resolve "./pages/organizers/events/event/stats" from "src/routes.jsx"
file: /Users/pc/code/vulos/cackle/web/src/routes.jsx
    at getRollupError (file:///Users/pc/code/vulos/cackle/web/node_modules/rollup/dist/es/shared/parseAst.js:317:41)
    at error (file:///Users/pc/code/vulos/cackle/web/node_modules/rollup/dist/es/shared/parseAst.js:313:42)
    at ModuleLoader.handleInvalidResolvedId (file:///Users/pc/code/vulos/cackle/web/node_modules/rollup/dist/es/shared/node-entry.js:22167:24)
    at file:///Users/pc/code/vulos/cackle/web/node_modules/rollup/dist/es/shared/node-entry.js:22127:26


To generate screenshots once the app boots:

```sh
make build      # builds web/ and the cackle binary with the UI embedded
./cackle --demo --addr :8087
npm run screenshots
```

This file is written automatically (and CI exits 0 despite the failure)
so a broken demo mode never blocks unrelated CI — see scripts/screenshots.mjs.

