import React from 'react';
import { formatMoney } from '@/lib/money';
import { cn } from '@/lib/utils';

// Fixed-order categorical hues for ticket-type identity (see index.css for
// the validated HSL values + the dataviz six-check notes). ORDER is the
// CVD-safety mechanism — never reassign by rank or cycle past 8; anything
// past the 8th ticket type folds into "Other" rather than generating a 9th
// hue (a generated hue is indistinguishable from an existing one under CVD).
const SERIES_SLOTS = 8;
const seriesColor = (i) => `hsl(var(--chart-${(i % SERIES_SLOTS) + 1}))`;

/**
 * Per-ticket-type sales bars: each type is a bar sized to its own sold/
 * capacity ratio, colored by fixed categorical slot, direct-labeled with
 * exact sold/capacity counts and revenue (never color-only — three of the
 * eight slots sit under 3:1 contrast on a light surface, so identity always
 * rides a visible label, not the color alone). A legend rides above once
 * there is more than one series, per the dataviz "always-present legend for
 * >=2 series" rule; a single ticket type needs no legend box.
 *
 * `byType` is Stats.ByType verbatim: [{ ticket_type_id, name, quantity_total,
 * sold, revenue_minor }] — every number here is real seeded/live data, never
 * invented; a type with 0 sold renders as a real, honest zero-width bar.
 */
export function TicketTypeBreakdown({ byType = [], currency }) {
    if (byType.length === 0) return null;

    const capacityOf = (t) => Math.max(t.quantity_total ?? 0, t.sold ?? 0);
    const maxCapacity = Math.max(1, ...byType.map(capacityOf));

    return (
        <div className="space-y-5">
            {byType.length > 1 && (
                <div className="flex flex-wrap gap-x-4 gap-y-1.5" role="list" aria-label="Ticket types">
                    {byType.map((t, i) => (
                        <span
                            key={t.ticket_type_id ?? t.name}
                            role="listitem"
                            className="inline-flex items-center gap-1.5 text-xs font-medium text-muted-foreground"
                        >
                            <span
                                className="h-2.5 w-2.5 shrink-0 rounded-full"
                                style={{ background: seriesColor(i) }}
                                aria-hidden="true"
                            />
                            {t.name}
                        </span>
                    ))}
                </div>
            )}

            <div className="space-y-4">
                {byType.map((t, i) => {
                    const sold = t.sold ?? 0;
                    // Bar length is relative to the LARGEST type's capacity across
                    // the event, so types are visually comparable to each other —
                    // not each normalized to its own 100%, which would make a
                    // 2-ticket type look as "full" as a 2,000-ticket one.
                    const widthPct = Math.max(sold > 0 ? 1.5 : 0, Math.round((sold / maxCapacity) * 100));
                    return (
                        <div key={t.ticket_type_id ?? t.name}>
                            <div className="mb-1.5 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5 text-sm">
                                <span className="flex items-center gap-2 font-medium text-foreground">
                                    <span
                                        className="h-2.5 w-2.5 shrink-0 rounded-full"
                                        style={{ background: seriesColor(i) }}
                                        aria-hidden="true"
                                    />
                                    {t.name}
                                </span>
                                <span className="text-muted-foreground">
                                    <span className="tabular-nums">{sold}</span>
                                    {t.quantity_total ? <span className="tabular-nums"> / {t.quantity_total}</span> : null} sold
                                    {' · '}
                                    {formatMoney(t.revenue_minor ?? 0, currency)}
                                </span>
                            </div>
                            <div className="h-3 w-full overflow-hidden rounded-full bg-chart-track">
                                <div
                                    className="h-full rounded-full transition-[width] duration-500 ease-out"
                                    style={{ width: `${widthPct}%`, background: seriesColor(i) }}
                                />
                            </div>
                        </div>
                    );
                })}
            </div>
        </div>
    );
}

/**
 * A single-ratio meter (one value against a limit, same-ramp fill/track —
 * per dataviz's "Meter" form) — used for both "sold of capacity" and
 * "admitted of sold". Never invents a percentage: a zero denominator renders
 * the honest empty copy instead of a NaN or a misleading 0%.
 */
export function RatioMeter({ label, value, of, valueLabel, ofLabel, emptyLabel, toneClassName }) {
    if (!of) {
        return (
            <div>
                <p className="mb-1.5 text-sm font-medium text-foreground">{label}</p>
                <div className="h-3 w-full rounded-full bg-chart-track" aria-hidden="true" />
                <p className="mt-1.5 text-xs text-muted-foreground">{emptyLabel}</p>
            </div>
        );
    }

    const pct = Math.max(0, Math.min(100, Math.round((value / of) * 100)));

    return (
        <div>
            <div className="mb-1.5 flex items-baseline justify-between gap-3 text-sm">
                <span className="font-medium text-foreground">{label}</span>
                <span className="tabular-nums text-muted-foreground">
                    {valueLabel ?? value} of {ofLabel ?? of} · {pct}%
                </span>
            </div>
            <div className="h-3 w-full overflow-hidden rounded-full bg-primary/10">
                <div
                    className={cn('h-full rounded-full bg-primary transition-[width] duration-500 ease-out', toneClassName)}
                    style={{ width: `${Math.max(pct, value > 0 ? 1.5 : 0)}%` }}
                />
            </div>
        </div>
    );
}
