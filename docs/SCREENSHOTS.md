# Screenshots

> **In plain English:** you don't need this page to *see* the screenshots —
> they're already placed next to the step they illustrate throughout
> [GETTING-STARTED.md](GETTING-STARTED.md) and the other chapters. This page
> is for regenerating them (if you're a contributor changing the UI) and for
> the full at-a-glance table of every surface captured, light and dark.

Every screenshot in this repository — the README gallery and this page — is
generated from the real, running application against seeded demo data.
Nothing here is a mockup.

## Regenerating

Run from the repo root — the `screenshots` script and its Playwright
devDependency live in the root `package.json`, not `web/`'s:

```bash
npm install
npx playwright install chromium   # one-time Chromium download
npm run screenshots
```

The screenshotter (`scripts/screenshots.mjs`) rebuilds `web/dist` and the Go
binary from the current tree, boots it with `--demo` on port **8087** (so it
doesn't collide with a Cackle instance you might already have running on
8080), and drives a real Chromium browser. It shoots **every surface in both
themes at both viewports** — 13 × 2 × 2 = 52 captures:

| | Viewport | File |
|---|---|---|
| Desktop | 1440×900, deviceScaleFactor 2 | `docs/screenshots/<surface>-<theme>.png` |
| Mobile | 390×844, deviceScaleFactor 2, touch + meta-viewport | `docs/screenshots/<surface>-<theme>-mobile.png` |

The desktop files carry **no viewport suffix**, and that is deliberate:
`site/index.html`, `README.md` and the docs chapters all reference those names
literally, so the bare `<surface>-<theme>` stem is the stable one and mobile is
a suffix appended after it. The landing page picks between the two at runtime
using the reader's own viewport, the same way it already picks a theme.

It then copies the hero shot to `docs/screenshots/hero.png` for the README
header. `landing` is the one surface captured as the FULL scrollable page
rather than just the viewport, so the flagship `hero.png` shows the demo events
listed below the hero, not just the hero on its own.

Every surface is walked top-to-bottom in viewport-sized steps before it is
shot. A single fast `scrollTo` outruns `IntersectionObserver`, so
reveal-on-scroll content never becomes visible and the capture comes out
empty — a capture artifact that has been mistaken for a bug in the app.

After the run the script asserts that **every** expected file exists, was
written by that run, and is not suspiciously small; that no two captures are
byte-identical (which is what a silent auth redirect, or a theme that never
flipped, actually looks like); and that no page logged a console error. It
still exits `0` regardless — see below — so those warnings are the evidence,
not the exit code.

If the app can't boot (missing build, port conflict, migration failure),
the script exits `0` and writes an explanatory
`docs/screenshots/README.md` instead of failing the whole run — screenshot
generation should never be the reason CI goes red for an unrelated change.

## Surfaces captured

Each row exists four times on disk: `-light.png`, `-dark.png`,
`-light-mobile.png` and `-dark-mobile.png`. The **File** column below writes
the desktop pair; append `-mobile` for the 390px pair.

| Surface | File | What it shows |
|---|---|---|
| Hero | `hero.png` | Copied from one of the surfaces below — whichever best represents the product at a glance, currently the **homepage** (`landing`), captured full-page. |
| Landing | `landing-{light,dark}.png` | The homepage, captured full-page (not just the viewport): hero, category filter, and the featured/upcoming events listing — sourced live from `GET /api/events`, so what's visible is always the real seeded catalogue, not a mockup. |
| Browse | `event-browse-{light,dark}.png` | The full events list — search, category filter. |
| Event detail | `event-detail-{light,dark}.png` | The public event page an attendee lands on: ticket types, pricing, availability. |
| Checkout | `checkout-{light,dark}.png` | Cart / checkout flow before handoff to the payment provider. |
| My tickets | `my-tickets-{light,dark}.png` | An attendee's ticket list. |
| Ticket | `ticket-qr-{light,dark}.png` | An attendee's issued ticket, QR code included — this QR *is* the signed capability described in [TICKET-FORMAT.md](TICKET-FORMAT.md). |
| Organiser home | `organiser-home-{light,dark}.png` | Organiser dashboard: their events, next-up event, quick actions. |
| Event editor | `event-editor-{light,dark}.png` | Organiser's event management view. |
| Ticket types | `ticket-types-{light,dark}.png` | Ticket types for an event, organiser side. |
| Attendees | `attendees-{light,dark}.png` | Attendee roster for an event. |
| Scanner | `scanner-{light,dark}.png` | The gate scanning view, mid-scan, showing an admission result. |
| Stats | `stats-{light,dark}.png` | Event analytics: sold/revenue/admitted, per-ticket-type breakdown, capacity and admission meters. |
| Settings | `settings-{light,dark}.png` | Organiser settings. |

## Adding a new surface

Add an entry to `SURFACES` in `scripts/screenshots.mjs` — the run already
loops it over both themes and both viewports, so one entry is four captures.
Give it a `discover` if its path contains a real id, so the shot survives a
change to the seed data. Reference the new file from the README gallery or
this page as appropriate — a screenshot that exists on disk but isn't linked
from anywhere is dead weight.
