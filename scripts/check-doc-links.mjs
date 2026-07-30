#!/usr/bin/env node
// Fail if a documentation link points at something that isn't there.
//
// Why this exists: the docs cross-reference each other constantly, and every
// one of those links is silently load-bearing — a reader following
// "see OFFLINE-GATES.md" and landing on a 404 learns nothing and trusts the
// rest less. Nothing checked them, so a renamed chapter or a deleted file
// broke links with no signal at all. Three separate things are verified here:
//
//   1. GitHub view — every relative link in the repo's own markdown resolves
//      to a file that exists, and every #fragment resolves to a heading that
//      exists in the target file.
//   2. Published site view — every relative link in the site/docs/ mirror is
//      one that site/docs.html's rewriteLinks() can turn into either a real
//      chapter jump or a real repo path. The mirror is served from a flat
//      directory by a runtime markdown viewer, so a link that works on
//      GitHub does NOT automatically work there.
//   3. Source comments — every `SOMETHING.md` a Go/JS source comment cites
//      names a document that exists. Passes 1 and 2 only look inside
//      markdown, so they never noticed that `BUILD-SPEC.md` (a file this
//      repo has never contained) was cited in 8 places across
//      internal/httpapi and web/src; that is the exact class of dead
//      pointer this pass exists to stop coming back.
//
// All three passes assert a minimum count, so a glob that silently matches
// nothing fails loudly instead of "passing".
//
// Usage: node scripts/check-doc-links.mjs

import { readFile, readdir } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { join, resolve, dirname, posix } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

// Floors, not targets: a run that checks suspiciously few links is a broken
// run, not a clean repo. Raise them as the docs grow.
const MIN_REPO_LINKS = 40;
const MIN_SITE_LINKS = 25;
const MIN_SOURCE_CITATIONS = 50;

// Chapter slug -> the directory its SOURCE lives in, mirroring the `base`
// field in site/docs.html's DOCS array. Kept in step with that file; the
// consistency check below fails if the two disagree.
const SITE_CHAPTER_BASE = {
    'overview.md': '',
    'getting-started.md': 'docs/',
    'architecture.md': 'docs/',
    'ticket-format.md': 'docs/',
    'offline-gates.md': 'docs/',
    'clustering.md': 'docs/',
    'federation.md': 'docs/',
    'api.md': 'docs/',
    'host-pages.md': 'docs/',
    'payments.md': 'docs/',
    'configuration.md': 'docs/',
    'self-hosting.md': 'docs/',
    'screenshots.md': 'docs/',
    'roadmap.md': '',
    'changelog.md': '',
};

const LINK_RE = /\[(?:[^\]\\]|\\.)*\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g;
const HEADING_RE = /^(#{1,6})\s+(.+?)\s*$/gm;

const problems = [];
const note = (file, msg) => problems.push(`${file}: ${msg}`);

/** GitHub's heading-anchor slug: lowercase, drop punctuation, spaces -> '-'. */
function headingAnchor(text) {
    const plain = text
        .replace(/`([^`]*)`/g, '$1') // inline code
        .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1') // links
        .replace(/[*_~]/g, '') // emphasis
        .trim();
    return plain
        .toLowerCase()
        .replace(/[^\p{L}\p{N} _-]/gu, '')
        .replace(/ /g, '-');
}

async function anchorsIn(absPath) {
    const src = await readFile(absPath, 'utf8');
    const out = new Set();
    for (const m of src.matchAll(HEADING_RE)) out.add(headingAnchor(m[2]));
    return out;
}

function linksIn(src) {
    const out = [];
    for (const m of src.matchAll(LINK_RE)) {
        const target = m[1];
        if (!target || /^(https?:|mailto:|data:)/i.test(target)) continue;
        out.push(target);
    }
    return out;
}

/** Resolve a repo-relative path containing ../ — same rule as docs.html. */
function resolveRepoPath(base, href) {
    const out = [];
    for (const part of (base + href).split('/')) {
        if (part === '' || part === '.') continue;
        if (part === '..') {
            out.pop();
            continue;
        }
        out.push(part);
    }
    return out.join('/');
}

async function markdownFilesUnder(dir) {
    const entries = await readdir(join(repoRoot, dir), { withFileTypes: true });
    return entries
        .filter((e) => e.isFile() && e.name.toLowerCase().endsWith('.md'))
        .map((e) => posix.join(dir, e.name));
}

// --- pass 1: the repo as GitHub renders it -----------------------------------

async function checkRepoLinks() {
    const files = [
        'README.md',
        'ROADMAP.md',
        'CHANGELOG.md',
        'CONTRIBUTING.md',
        'SECURITY.md',
        ...(await markdownFilesUnder('docs')),
        'internal/tickets/README.md',
        'docs/screenshots/README.md',
    ].filter((f) => existsSync(join(repoRoot, f)));

    const anchorCache = new Map();
    let checked = 0;

    for (const file of files) {
        const src = await readFile(join(repoRoot, file), 'utf8');
        const fileDir = posix.dirname(file);

        for (const target of linksIn(src)) {
            const [rawPath, fragment] = target.split('#');
            checked++;

            if (!rawPath) {
                // Same-document anchor.
                if (!anchorCache.has(file)) anchorCache.set(file, await anchorsIn(join(repoRoot, file)));
                if (!anchorCache.get(file).has(fragment)) {
                    note(file, `same-document anchor "#${fragment}" matches no heading`);
                }
                continue;
            }

            const repoPath = resolveRepoPath(fileDir === '.' ? '' : fileDir + '/', rawPath);
            const abs = join(repoRoot, repoPath);
            if (!existsSync(abs)) {
                note(file, `link "${target}" -> ${repoPath} does not exist`);
                continue;
            }
            if (fragment && repoPath.toLowerCase().endsWith('.md')) {
                if (!anchorCache.has(repoPath)) anchorCache.set(repoPath, await anchorsIn(abs));
                if (!anchorCache.get(repoPath).has(fragment)) {
                    note(file, `link "${target}" -> ${repoPath} has no heading anchor "#${fragment}"`);
                }
            }
        }
    }

    if (checked < MIN_REPO_LINKS) {
        note('scripts/check-doc-links.mjs', `only ${checked} repo links checked, expected at least ${MIN_REPO_LINKS} — the file list or link regex is broken`);
    }
    return checked;
}

// --- pass 2: the mirror as site/docs.html serves it --------------------------

async function checkSiteLinks() {
    const viewer = await readFile(join(repoRoot, 'site', 'docs.html'), 'utf8');

    // The viewer's own slug->base table must agree with this script's copy,
    // or pass 2 is checking a resolution rule the site does not use.
    for (const [chapter, base] of Object.entries(SITE_CHAPTER_BASE)) {
        const slug = chapter.replace(/\.md$/, '');
        const entry = new RegExp(`"slug"\\s*:\\s*"${slug}"[^}]*"base"\\s*:\\s*"${base}"`);
        if (!entry.test(viewer)) {
            note('site/docs.html', `DOCS entry for "${slug}" is missing or does not declare base "${base}"`);
        }
    }

    const slugs = new Set(Object.keys(SITE_CHAPTER_BASE).map((c) => c.replace(/\.md$/, '')));
    let checked = 0;

    for (const file of await markdownFilesUnder(posix.join('site', 'docs'))) {
        const chapter = posix.basename(file);
        const base = SITE_CHAPTER_BASE[chapter];
        if (base === undefined) {
            note(file, 'chapter is not listed in site/docs.html — the viewer cannot serve it');
            continue;
        }
        const src = await readFile(join(repoRoot, file), 'utf8');

        for (const target of linksIn(src)) {
            const [rawPath] = target.split('#');
            if (!rawPath) continue; // same-document anchor; the viewer leaves it alone
            checked++;

            const repoPath = resolveRepoPath(base, rawPath);
            const slug = (repoPath.split('/').pop() || '').toLowerCase().replace(/\.md$/, '');

            // Either it becomes an in-page chapter jump...
            if (/\.md$/i.test(repoPath) && slugs.has(slug)) continue;
            // ...or a GitHub link, which had better point at a real path.
            if (!existsSync(join(repoRoot, repoPath))) {
                note(file, `link "${target}" resolves to ${repoPath}, which does not exist in the repo`);
            }
        }
    }

    if (checked < MIN_SITE_LINKS) {
        note('scripts/check-doc-links.mjs', `only ${checked} site links checked, expected at least ${MIN_SITE_LINKS} — the mirror or link regex is broken`);
    }
    return checked;
}

// --- pass 3: documents cited by source comments ------------------------------

// Source trees whose comments are checked. site/docs.html and
// scripts/sync-docs.mjs are deliberately NOT here: their `.md` strings are
// the site viewer's own chapter table and the mirror generator's slug list,
// which pass 2 already validates as a table (and which name flat mirror
// slugs, not repo paths).
const SOURCE_DIRS = ['cmd', 'internal', 'web/src'];
const SOURCE_EXTS = /\.(go|js|jsx|mjs)$/;

// A bare `NAME.md` with no directory is looked for in each of these, in
// order — the way a reader would actually go hunting for it.
const BARE_NAME_DIRS = ['', 'docs/', 'site/docs/'];

// Documents that genuinely exist, but in the SIBLING patala checkout rather
// than this repo — cited by internal/payments/patala.go, which is the one
// file here that talks about patala's own internals (see its build comment;
// `-tags patala` requires ../patala checked out). Listed explicitly so that
// a citation of anything else unresolvable still fails.
const SIBLING_PATALA_DOCS = new Set([
    'PATALA.md',
    'PORTING.md',
    'patala-fiat/PORTING.md',
    'patala-go/README.md',
    'docs/compensating-payments.md',
]);

// Specifications that live in ANOTHER PROJECT entirely and are cited by name
// because that is the only honest way to cite them — KOTVA's sync capability
// document, referenced by internal/scan/substrate, which implements the
// mapping that document specifies. It is not vendored here (only the compiled
// engine is, inside the Go module) so there is no repo path it could resolve
// to, and inventing a local stub to satisfy this pass would be worse than
// naming the real source.
//
// Listed EXACTLY, and one spelling only, for the same reason as the patala set
// above: a typo, a renamed section file, or a citation of some other external
// document still fails. Cite it as `substrate/SYNC.md`, matching the path
// inside the KOTVA repository.
const EXTERNAL_SPEC_DOCS = new Set(['substrate/SYNC.md']);

// `[A-Za-z0-9_]` first char keeps this off `*.md` globs and bare `.md`
// extensions in code.
const SOURCE_CITATION_RE = /[A-Za-z0-9_][A-Za-z0-9_./-]*\.md\b/g;

// Case-EXACT existence: macOS and Windows would happily resolve
// `docs/api.md` to `docs/API.md`, and then CI on Linux (and the published
// site) would 404. Every path is checked segment by segment against the real
// directory listing.
const dirCache = new Map();
async function entriesOf(dir) {
    if (!dirCache.has(dir)) {
        try {
            dirCache.set(dir, new Set(await readdir(join(repoRoot, dir))));
        } catch {
            dirCache.set(dir, new Set());
        }
    }
    return dirCache.get(dir);
}
async function existsExact(repoPath) {
    const parts = repoPath.split('/').filter(Boolean);
    let dir = '';
    for (let i = 0; i < parts.length; i++) {
        if (!(await entriesOf(dir)).has(parts[i])) return false;
        dir = dir ? `${dir}/${parts[i]}` : parts[i];
    }
    return true;
}

async function sourceFiles() {
    // git ls-files, not a hand-rolled walk: it skips node_modules and build
    // output for free. `--others --exclude-standard` includes not-yet-
    // committed files too, so a brand-new source file's citations are
    // checked on the run BEFORE it is committed, not the run after. Files
    // staged for deletion are filtered out below.
    const listed = execFileSync('git', ['ls-files', '-z', '--cached', '--others', '--exclude-standard', ...SOURCE_DIRS], {
        cwd: repoRoot,
        encoding: 'utf8',
        maxBuffer: 32 * 1024 * 1024,
    })
        .split('\0')
        .filter(Boolean)
        .filter((f) => SOURCE_EXTS.test(f));
    const out = [];
    for (const f of listed) if (existsSync(join(repoRoot, f))) out.push(f);
    return out;
}

async function checkSourceCitations() {
    const files = await sourceFiles();
    if (files.length === 0) {
        note(
            'scripts/check-doc-links.mjs',
            `git ls-files matched no source files under ${SOURCE_DIRS.join(', ')} — this pass would verify nothing`,
        );
        return 0;
    }

    let checked = 0;
    for (const file of files) {
        const src = await readFile(join(repoRoot, file), 'utf8');
        const fileDir = posix.dirname(file);
        const reported = new Set();

        for (const m of src.matchAll(SOURCE_CITATION_RE)) {
            const cited = m[0];
            checked++;
            if (reported.has(cited)) continue;

            if (SIBLING_PATALA_DOCS.has(cited) || EXTERNAL_SPEC_DOCS.has(cited)) continue;

            const candidates = cited.includes('/')
                ? [resolveRepoPath('', cited), resolveRepoPath(fileDir + '/', cited)]
                : [
                      ...BARE_NAME_DIRS.map((d) => d + cited),
                      resolveRepoPath(fileDir + '/', cited),
                  ];

            let found = false;
            for (const candidate of candidates) {
                if (await existsExact(candidate)) {
                    found = true;
                    break;
                }
            }
            if (!found) {
                reported.add(cited);
                note(file, `comment cites "${cited}", which is not a document in this repo (looked in ${candidates.join(', ')})`);
            }
        }
    }

    if (checked < MIN_SOURCE_CITATIONS) {
        note('scripts/check-doc-links.mjs', `only ${checked} source-comment citations checked, expected at least ${MIN_SOURCE_CITATIONS} — the file list or citation regex is broken`);
    }
    return checked;
}

const repoChecked = await checkRepoLinks();
const siteChecked = await checkSiteLinks();
const sourceChecked = await checkSourceCitations();

if (problems.length) {
    console.error('check-doc-links: broken documentation links\n');
    for (const p of problems) console.error(`  ${p}`);
    console.error(`\n${problems.length} problem(s). Fix the link or the target.`);
    process.exit(1);
}

console.log(
    `check-doc-links: OK — ${repoChecked} repo links, ${siteChecked} site-mirror links and ${sourceChecked} source-comment citations resolve.`,
);
