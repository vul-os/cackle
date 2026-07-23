#!/usr/bin/env node
// Copy the repo's markdown docs into site/docs/ for the docs viewer.
//
// Why this exists: the docs viewer (site/docs.html) fetches markdown at
// runtime, so the site needs its own copy of every chapter. That copy used to
// be made by hand and the published docs quietly drifted from the ones in the
// repo — site/docs/payments.md still described the pre-patala payment system
// long after docs/PAYMENTS.md had been rewritten, and readers of the site were
// served stale (in one case actively wrong) text with no signal it was stale.
// Here the copy is a script, it is the only way the files get there, and
// `--check` fails CI if someone edits docs/ without re-running it.
//
// Images referenced by screenshots.md are NOT copied here: they live in
// site/screenshots/ and the viewer rewrites <img> paths at render time
// (site/docs.html). They are mirrored from docs/screenshots/ separately.
//
// Usage:
//   node scripts/sync-docs.mjs           copy docs/*.md (+ roadmap/changelog) -> site/docs/*.md
//   node scripts/sync-docs.mjs --check   exit 1 if the copies are out of date
//   node scripts/sync-docs.mjs --quiet   only report changes and errors

import { readdir, readFile, writeFile, mkdir, rm } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { join, resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const docsDir = join(repoRoot, 'docs');
const siteDocsDir = join(repoRoot, 'site', 'docs');

// Root-level markdown the docs viewer also serves, keyed by repo-root-relative
// source path; the value is the slug it lands under (see site/docs.html's nav).
const EXTRA = {
  'ROADMAP.md': 'roadmap',
  'CHANGELOG.md': 'changelog',
};

// Site-authored chapters that have NO source under docs/ (hand-written for the
// marketing site only). They must never be pruned as "orphaned copies".
const SITE_ONLY = new Set(['overview.md']);

const args = new Set(process.argv.slice(2));
const check = args.has('--check');
const quiet = args.has('--quiet');

/** `GETTING-STARTED.md` -> `getting-started.md` */
const slugFor = (name) => name.replace(/\.md$/i, '').toLowerCase() + '.md';

const log = (...a) => { if (!quiet) console.log(...a); };

async function collect() {
  const out = [];

  // Only docs/*.md itself — not subdirectories (docs/assets/, docs/screenshots/
  // are assets, not chapters).
  const entries = await readdir(docsDir, { withFileTypes: true });
  for (const e of entries) {
    if (!e.isFile() || !e.name.toLowerCase().endsWith('.md')) continue;
    out.push({ from: join(docsDir, e.name), to: join(siteDocsDir, slugFor(e.name)) });
  }

  for (const [rel, slug] of Object.entries(EXTRA)) {
    const from = join(repoRoot, rel);
    if (existsSync(from)) out.push({ from, to: join(siteDocsDir, `${slug}.md`) });
  }

  return out.sort((a, b) => a.to.localeCompare(b.to));
}

async function main() {
  if (!existsSync(docsDir)) {
    console.error(`sync-docs: no docs/ directory at ${docsDir}`);
    process.exit(1);
  }

  const files = await collect();
  if (files.length === 0) {
    console.error('sync-docs: docs/ contains no markdown files');
    process.exit(1);
  }

  await mkdir(siteDocsDir, { recursive: true });

  const expected = new Set(files.map((f) => f.to));
  const stale = [];
  let changed = 0;

  // Remove copies whose source has gone away, so a deleted chapter cannot
  // linger on the site — but never touch the site-only chapters.
  for (const e of await readdir(siteDocsDir, { withFileTypes: true })) {
    if (!e.isFile() || !e.name.toLowerCase().endsWith('.md')) continue;
    if (SITE_ONLY.has(e.name)) continue;
    const path = join(siteDocsDir, e.name);
    if (expected.has(path)) continue;
    stale.push(path);
    if (!check) await rm(path);
  }

  for (const { from, to } of files) {
    const src = await readFile(from, 'utf8');
    const dst = existsSync(to) ? await readFile(to, 'utf8') : null;
    if (dst === src) continue;
    changed++;
    if (!check) await writeFile(to, src);
    log(`  ${dst === null ? 'new  ' : 'sync '} ${to.slice(repoRoot.length + 1)}`);
  }

  for (const path of stale) {
    log(`  ${check ? 'stale' : 'rm   '} ${path.slice(repoRoot.length + 1)}`);
  }

  if (check && (changed || stale.length)) {
    console.error(
      `\nsync-docs: site/docs is out of date (${changed} to copy, ${stale.length} stale).\n` +
      'Run `npm run docs:sync` and commit the result.'
    );
    process.exit(1);
  }

  log(
    changed || stale.length
      ? `sync-docs: ${files.length} chapters, ${changed} written, ${stale.length} removed.`
      : `sync-docs: ${files.length} chapters already up to date.`
  );
}

main().catch((err) => {
  console.error('sync-docs:', err.message);
  process.exit(1);
});
