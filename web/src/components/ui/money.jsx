import React from 'react';
import { cn } from '@/lib/utils';
import { formatMoney } from '@/lib/money';

// The one way money is rendered in this app.
//
// `lib/money.js` already owns the hard part — minor units, real ISO-4217
// exponents, the narrowSymbol trick that stops an en-US browser printing
// "ZAR 450.00" instead of "R 450.00". This component owns what that file
// cannot: how the result SITS on the page.
//
// # Why this exists rather than `{formatMoney(...)}` inline
//
// Cackle prices events in whatever currency the organiser chose, and a payouts
// table can hold several at once. Those strings have wildly different widths —
// "¥4,500", "R 1,250.00", "35.939 KWD" — and proportional digits make even one
// currency jump: in Figtree a "1" is narrower than a "0", so a running total
// ticking 19 -> 20 visibly shifts every character to its left. Money that moves
// while you read it is money you re-read.
//
// Two fixes, and both have to be present:
//   1. `tnum` — tabular figures, so every digit takes the same advance width.
//      Figtree ships the feature but ALSO ships `pnum`, and proportional is
//      what you get by default, so it has to be asked for. See `.tnum` in
//      index.css.
//   2. Right alignment in columns — tabular figures fix digit width, not
//      symbol width, so "R " and "¥" still shift the start of the string. In a
//      column what must line up is the last digit, not the first character.
//
// `<MoneyCell>` applies both plus `whitespace-nowrap`, because a currency
// string wrapping mid-amount is its own kind of wrong.

/**
 * Money renders an integer minor-unit amount in its own currency, with
 * tabular figures.
 *
 * @param {object} props
 * @param {number} props.minor integer minor units (price_minor, total_minor…)
 * @param {string} props.currency ISO-4217 alpha-3
 * @param {string} [props.locale] override the browser's formatting locale
 * @param {string} [props.className]
 * @param {React.ElementType} [props.as] element to render, default `span`
 */
export function Money({ minor, currency, locale, className, as: Tag = 'span', ...rest }) {
    const formatted = formatMoney(minor, currency, locale ? { locale } : undefined);
    return (
        <Tag className={cn('tnum', className)} {...rest}>
            {formatted}
        </Tag>
    );
}

/**
 * MoneyCell is Money for a table column: right-aligned and non-wrapping, so a
 * column of mixed currencies keeps its last digits on one axis and no row
 * changes width when a value does.
 *
 * Renders a `<td>` by default; pass `as="div"` inside a non-table layout.
 */
export function MoneyCell({ minor, currency, locale, className, as: Tag = 'td', ...rest }) {
    return (
        <Money
            as={Tag}
            minor={minor}
            currency={currency}
            locale={locale}
            className={cn('whitespace-nowrap text-right', className)}
            {...rest}
        />
    );
}

/**
 * MoneyHeading is the large display treatment — a revenue figure at the top of
 * a stats page, an order total at checkout. Same tabular guarantee, sized for
 * a number that is the point of the screen rather than a cell in a table.
 */
export function MoneyHeading({ minor, currency, locale, className, ...rest }) {
    return (
        <Money
            minor={minor}
            currency={currency}
            locale={locale}
            className={cn('font-display text-display-sm font-bold tracking-tight', className)}
            {...rest}
        />
    );
}

export default Money;
