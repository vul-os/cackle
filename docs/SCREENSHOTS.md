# Screenshots

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

The screenshotter (`scripts/screenshots.mjs`) builds the Go binary, boots it
with `--demo` on port **8087** (so it doesn't collide with a Cackle instance
you might already have running on 8080), and drives a real Chromium browser
at 1440×900, deviceScaleFactor 2. It shoots each surface in both light and
dark mode to `docs/screenshots/<surface>-<theme>.png`, then copies the hero
shot to `docs/screenshots/hero.png` for the README header.

If the app can't boot (missing build, port conflict, migration failure),
the script exits `0` and writes an explanatory
`docs/screenshots/README.md` instead of failing the whole run — screenshot
generation should never be the reason CI goes red for an unrelated change.

## Surfaces captured

| Surface | File | What it shows |
|---|---|---|
| Hero | `hero.png` | Copied from one of the surfaces below — whichever best represents the product at a glance, currently the **homepage** (`landing`). |
| Landing | `landing-{light,dark}.png` | The homepage: hero, category filter, featured/upcoming events. |
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

Add the route and a shot call in `scripts/screenshots.mjs`, following the
existing pattern (navigate, wait for the surface to settle, shoot light,
toggle theme, shoot dark). Reference the new file from the README gallery
or this page as appropriate — a screenshot that exists on disk but isn't
linked from anywhere is dead weight.
