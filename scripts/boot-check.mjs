#!/usr/bin/env node
// CI boot guard — "a green build proves nothing about whether the app RUNS."
//
// Several sibling repos shipped a blank screen behind a green `go build` +
// `npm run build`. This script closes that gap for Cackle: it boots the
// ALREADY-BUILT `cackle` binary (see .github/workflows/ci.yml — `make build`
// runs first) with --demo, points a real headless Chromium at it, and fails
// the build if:
//   - the server never becomes reachable,
//   - the page throws an uncaught exception / unhandled rejection,
//   - #root ends up empty (nothing mounted),
//   - the browser console logged an `error`-level message.
//
// Usage:
//   ./cackle --demo &            (this script starts it for you instead)
//   node scripts/boot-check.mjs [--bin ./cackle] [--port 8089] [--path /]
//
// Exit code is the ONLY thing CI reads: non-zero fails the job. Unlike
// scripts/screenshots.mjs (which must exit 0 so a broken demo mode never
// blocks unrelated CI), this script exists specifically to be strict.
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import path from 'node:path';

function arg(name, fallback) {
  const i = process.argv.indexOf(`--${name}`);
  return i !== -1 && process.argv[i + 1] ? process.argv[i + 1] : fallback;
}

const BIN = path.resolve(arg('bin', './cackle'));
const PORT = arg('port', '8089');
const ROUTE = arg('path', '/');
const BASE = `http://127.0.0.1:${PORT}`;

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

async function waitForHTTP(url, maxMs = 30_000) {
  const deadline = Date.now() + maxMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url, { signal: AbortSignal.timeout(1_500) });
      if (res.status < 600) return true;
    } catch {
      /* not yet */
    }
    await sleep(500);
  }
  return false;
}

async function main() {
  if (!existsSync(BIN)) {
    console.error(`boot-check: binary not found at ${BIN} — build it first (\`make build\`)`);
    process.exit(1);
  }

  console.log(`boot-check: starting ${BIN} --demo on :${PORT}`);
  const proc = spawn(BIN, ['--demo', '--addr', `:${PORT}`], {
    stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...process.env, CACKLE_DB: ':memory:' },
  });
  let serverLog = '';
  proc.stdout.on('data', (d) => (serverLog += d.toString()));
  proc.stderr.on('data', (d) => (serverLog += d.toString()));

  let exitCode = 1;
  try {
    const up = await waitForHTTP(`${BASE}/`, 30_000);
    if (!up) {
      console.error('boot-check: server never became reachable within 30s');
      console.error('--- server output ---\n' + serverLog);
      process.exit(1);
    }

    const browser = await chromium.launch({ headless: true });
    const page = await browser.newPage();

    const pageErrors = [];
    const consoleErrors = [];
    page.on('pageerror', (err) => pageErrors.push(String(err)));
    page.on('console', (msg) => {
      if (msg.type() === 'error') consoleErrors.push(msg.text());
    });

    await page.goto(`${BASE}${ROUTE}`, { waitUntil: 'domcontentloaded', timeout: 20_000 });
    try {
      await page.waitForLoadState('networkidle', { timeout: 8_000 });
    } catch {
      await page.waitForTimeout(2_000);
    }
    // Give the SPA a moment to mount past the initial render.
    await page.waitForTimeout(1_000);

    const rootHTML = await page.evaluate(() => document.getElementById('root')?.innerHTML ?? null);
    const bodyText = await page.evaluate(() => document.body.innerText ?? '');

    await browser.close();

    let ok = true;
    if (rootHTML === null) {
      console.error(`boot-check: FAIL — no #root element found in the served page`);
      ok = false;
    } else if (rootHTML.trim().length === 0) {
      console.error(`boot-check: FAIL — #root is present but EMPTY (nothing mounted)`);
      ok = false;
    } else if (bodyText.trim().length === 0) {
      console.error(`boot-check: FAIL — page rendered no visible text`);
      ok = false;
    }
    if (pageErrors.length) {
      console.error(`boot-check: FAIL — ${pageErrors.length} uncaught exception(s):`);
      for (const e of pageErrors) console.error('  ' + e);
      ok = false;
    }
    if (consoleErrors.length) {
      console.error(`boot-check: FAIL — ${consoleErrors.length} console.error() call(s):`);
      for (const e of consoleErrors) console.error('  ' + e);
      ok = false;
    }

    if (ok) {
      console.log(`boot-check: OK — ${BASE}${ROUTE} rendered ${rootHTML.length} chars into #root, no errors`);
      exitCode = 0;
    } else {
      console.error('--- server output ---\n' + serverLog);
      exitCode = 1;
    }
  } catch (err) {
    console.error('boot-check: FAIL — unexpected error:', err);
    console.error('--- server output ---\n' + serverLog);
    exitCode = 1;
  } finally {
    proc.kill('SIGTERM');
  }

  process.exit(exitCode);
}

main();
