import React from 'react';
import { cn } from '@/lib/utils';
import logo from '/cackle.svg';

// The one place Cackle's identity is drawn.
//
// Before this file existed the mark was hand-rolled at every site that needed
// it — the top bar spelled it lowercase with a `--primary-emphasis` dot, the
// visitor header and the landing footer each imported the SVG separately, and
// the mini-site declared its own copy in a <style> block. Four renderings of
// one mark is four things to forget when the brand moves, and the brand just
// moved. So: one component, one import of the logo, no literals.
//
// # The rules this component encodes
//
//   Word   `Cackle`, Figtree 800, letter-spacing -0.03em. INK on a light
//          ground, WHITE on a dark one.
//   Dot    a full stop in RED #FF4848, in BOTH themes. The dot is the mark's
//          echo in type, so it is identity and does not follow the OS theme.
//          It renders from `--brand`, which is declared once in :root and
//          deliberately not redefined under .dark (contrast.test.js asserts
//          that, so this cannot drift).
//   Tile   brand/logo.svg, copied byte-identical to web/public/cackle.svg.
//          APPROVED AND FINAL — copied outward, never redrawn.
//
// The dot must never be the only red on a screen. `<BrandLockup>` is the
// default export shape for exactly that reason: it pairs the wordmark with
// the tile, so the red always has a second occurrence anchoring it.

/**
 * TONES maps a rendering context to the word's colour. The dot is absent from
 * this table on purpose — it is red in every tone.
 *
 * `auto` follows the page's own foreground token, which is INK in light and
 * white in dark. The other two are for surfaces that pin their own ground
 * regardless of theme: the console chrome (always ink) and the gate screen.
 */
const TONES = {
    auto: 'text-foreground',
    onDark: 'text-sidebar-foreground',
    // `text-brand-2` is the registered utility for INK-as-identity (see
    // tailwind.config.js). It was an arbitrary `text-[hsl(var(--brand-2))]`,
    // which resolves to the same pixels but is a bracket escape hatch: it
    // works whether or not the token is real, so a renamed or deleted token
    // fails silently to `color: hsl()` and the word renders inheriting
    // whatever is around it. A registered utility does not exist if its
    // token does not, so the build is what notices.
    onLight: 'text-brand-2',
};

/**
 * SIZES pairs a type step with the tile size that balances it. Every step is
 * at or above the suite's 12px floor.
 */
const SIZES = {
    xs: { text: 'text-sm', tile: 'h-6 w-6', gap: 'gap-1.5' },
    sm: { text: 'text-lg', tile: 'h-7 w-7', gap: 'gap-2' },
    md: { text: 'text-2xl', tile: 'h-8 w-8', gap: 'gap-2' },
    lg: { text: 'text-3xl', tile: 'h-10 w-10', gap: 'gap-2.5' },
    xl: { text: 'text-display-sm sm:text-display-md', tile: 'h-12 w-12 sm:h-14 sm:w-14', gap: 'gap-3' },
};

/**
 * stepFor resolves a size name to its step, falling back to the medium one.
 *
 * Bracket notation rather than dot access, because the medium step's key
 * collides with a file extension: scripts/check-doc-links.mjs reads any
 * bare identifier followed by that extension, anywhere in the tree, as a
 * reference to a markdown document, and fails the build hunting for a file
 * that was never meant to exist.
 *
 * @param {string} size
 */
const stepFor = (size) => SIZES[size] ?? SIZES['md'];

/**
 * Wordmark renders `Cackle.` — the type half of the identity.
 *
 * @param {object} props
 * @param {'xs'|'sm'|'md'|'lg'|'xl'} [props.size]
 * @param {'auto'|'onDark'|'onLight'} [props.tone] which ground it sits on
 * @param {boolean} [props.hideWordBelowSm] collapse to the tile on narrow
 *   screens (the console top bar does this to protect the 390px layout)
 */
export function Wordmark({ size = 'md', tone = 'auto', hideWordBelowSm = false, className, ...props }) {
    const step = stepFor(size);
    return (
        <span
            className={cn('font-display font-extrabold leading-none', step.text, TONES[tone] ?? TONES.auto, className)}
            style={{ letterSpacing: '-0.03em' }}
            {...props}
        >
            {/* Below sm the word is still in the accessibility tree — it is
                the product's name, and a screen reader should read it even
                when the layout has no room to draw it. */}
            <span className={hideWordBelowSm ? 'sr-only sm:not-sr-only' : undefined}>Cackle</span>
            {/* aria-hidden: the dot is a graphic device. Spoken, it turns the
                brand into a sentence fragment. */}
            <span className="text-brand" aria-hidden="true">
                .
            </span>
        </span>
    );
}

/**
 * LogoTile renders brand/logo.svg. The single import site for the mark in the
 * whole app — a second `import logo from '/cackle.svg'` anywhere else is the
 * beginning of the drift this file exists to end.
 *
 * Decorative by default: it is almost always beside the wordmark, and an
 * `alt="Cackle"` next to the word "Cackle" makes a screen reader say the
 * product's name twice. Pass `alt` where the tile stands alone.
 *
 * @param {object} props
 * @param {'xs'|'sm'|'md'|'lg'|'xl'} [props.size]
 * @param {string} [props.alt]
 */
export function LogoTile({ size = 'md', alt = '', className, ...props }) {
    const step = stepFor(size);
    return (
        <img
            src={logo}
            alt={alt}
            width={128}
            height={128}
            // width/height above are the SVG's intrinsic box, so the element
            // reserves its space before the file loads and nothing beside it
            // shifts. The classes are what actually size it.
            className={cn('shrink-0 select-none', step.tile, className)}
            {...props}
        />
    );
}

/**
 * BrandLockup is the tile and the wordmark together, and is what almost every
 * caller wants: it guarantees the wordmark's red dot is never the only red on
 * screen, which is the one composition rule the mark has.
 *
 * @param {object} props
 * @param {'xs'|'sm'|'md'|'lg'|'xl'} [props.size]
 * @param {'auto'|'onDark'|'onLight'} [props.tone]
 * @param {boolean} [props.hideWordBelowSm]
 */
export function BrandLockup({ size = 'md', tone = 'auto', hideWordBelowSm = false, className, ...props }) {
    const step = stepFor(size);
    return (
        <span className={cn('inline-flex items-center', step.gap, className)} {...props}>
            <LogoTile size={size} />
            <Wordmark size={size} tone={tone} hideWordBelowSm={hideWordBelowSm} />
        </span>
    );
}

export default BrandLockup;
