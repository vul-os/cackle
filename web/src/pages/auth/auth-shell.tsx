import type { CSSProperties, ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { motion, useReducedMotion } from 'framer-motion';
import { AlertCircle, type LucideIcon } from 'lucide-react';
import { Separator } from '@/components/ui/separator';
import { BrandLockup } from '@/components/brand/wordmark';

interface AuthShellPoint {
    icon: LucideIcon;
    text: string;
}

interface AuthShellProps {
    eyebrow?: ReactNode;
    title: ReactNode;
    description?: ReactNode;
    points?: AuthShellPoint[];
    formHeading?: ReactNode;
    formSubheading?: ReactNode;
    children?: ReactNode;
}

/**
 * The shared frame for sign-in, sign-up, forgot-password and update-password.
 *
 * The shape is the product's own object: a ticket has a stub you keep and a
 * part that gets used. The rail (always ink, both themes — `bg-sidebar` is
 * the same "persistent shell" token the top bar and side nav already stand
 * on, not a new surface) carries the identity and the one honest thing worth
 * saying about *this* screen. The perforation between rail and form is a
 * genuine stub division, not decoration: the rail is what you already have,
 * the form is what gets you in.
 *
 * Below `sm` the rail's supporting points collapse — there is real content on
 * a 390px screen and it is the form, not the pitch. Below `md` the whole
 * thing stacks (rail on top, torn horizontally); at `md` and up it runs side
 * by side, torn vertically. Two live `<Separator variant="perforated">`
 * elements handle this — one hidden past `md`, one hidden before it — rather
 * than one element switching orientation by media query, because the
 * primitive's orientation is a prop, not a CSS trick.
 */
export function AuthShell({ eyebrow, title, description, points = [], formHeading, formSubheading, children }: AuthShellProps) {
    const prefersReducedMotion = useReducedMotion();

    return (
        <div className="relative min-h-screen overflow-hidden bg-background">
            {/* Atmosphere only — never content, always aria-hidden. The same
                brand-glow + dot-grid language the landing hero runs on, at a
                fraction of the strength since this page has real work to do.

                Both boxes are narrower than the smallest supported viewport
                (390px) and pinned to a non-negative inset, so their own
                geometry never crosses the viewport edge — the `blur-[…]`
                filter paints well past the box without moving it, which is
                where the visual bleed actually comes from. `overflow-hidden`
                on the wrapper is defence in depth, not the thing doing the
                work: the brief is explicit that hidden/clip never forgives an
                overflowing child, only a genuine scroll container does, so
                the geometry has to be honest on its own. */}
            <div className="pointer-events-none absolute inset-0 overflow-hidden" aria-hidden="true">
                <div className="absolute -top-16 right-0 h-64 w-64 rounded-full bg-primary/10 blur-[110px]" />
                <div className="absolute -bottom-10 left-0 h-56 w-56 rounded-full bg-primary/5 blur-[90px]" />
            </div>

            <div className="relative flex min-h-screen flex-col items-center justify-center px-4 py-10 sm:px-6 sm:py-14">
                <motion.div
                    initial={prefersReducedMotion ? { opacity: 1, y: 0 } : { opacity: 0, y: 16 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ duration: 0.4, ease: [0.2, 0, 0, 1] }}
                    className="w-full max-w-4xl"
                >
                    <div className="grid grid-cols-1 overflow-hidden rounded-2xl border border-border bg-card shadow-floating md:grid-cols-[18rem_1px_1fr] lg:grid-cols-[22rem_1px_1fr]">
                        {/* The stub: identity + the one thing worth knowing here. */}
                        <div className="flex flex-col gap-8 bg-sidebar px-6 py-8 text-sidebar-foreground sm:px-8 sm:py-10 md:py-10 lg:py-12">
                            {/* -m-2 cancels the p-2 visually — the logo sits
                                exactly where it would with no padding at all —
                                while still growing the link's own box to a
                                44px tap target. Home's the only place this
                                mark is also a control; everywhere else
                                (top-bar, side-nav) it's already sized past
                                44px by its own layout. */}
                            <Link
                                to="/"
                                className="-m-2 inline-flex w-fit rounded-md p-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-sidebar-background"
                            >
                                <BrandLockup tone="onDark" size="md" />
                            </Link>

                            <div className="space-y-3">
                                {eyebrow && (
                                    <p className="text-xs font-semibold uppercase tracking-wider text-sidebar-foreground/60">
                                        {eyebrow}
                                    </p>
                                )}
                                <h1 className="text-balance text-display-sm font-extrabold leading-tight text-sidebar-foreground">
                                    {title}
                                </h1>
                                {description && (
                                    <p className="text-pretty text-sm leading-relaxed text-sidebar-foreground/70">{description}</p>
                                )}
                            </div>

                            {points.length > 0 && (
                                <ul className="hidden space-y-3 border-t border-sidebar-border pt-6 sm:block">
                                    {points.map(({ icon: Icon, text }) => (
                                        <li key={text} className="flex items-start gap-2.5 text-sm text-sidebar-foreground/75">
                                            <Icon className="mt-0.5 h-4 w-4 shrink-0 text-primary" aria-hidden="true" />
                                            <span>{text}</span>
                                        </li>
                                    ))}
                                </ul>
                            )}
                        </div>

                        <Separator
                            variant="perforated"
                            className="md:hidden"
                            style={{ '--notch': 'var(--card)' } as CSSProperties}
                            aria-hidden="true"
                        />
                        <Separator
                            variant="perforated"
                            orientation="vertical"
                            className="hidden md:block"
                            style={{ '--notch': 'var(--card)' } as CSSProperties}
                            aria-hidden="true"
                        />

                        {/* The part that gets used. */}
                        <div className="flex flex-col justify-center px-6 py-8 sm:px-8 sm:py-10 md:py-10 lg:px-10 lg:py-12">
                            {(formHeading || formSubheading) && (
                                <div className="mb-6 space-y-1.5">
                                    {formHeading && (
                                        <h2 className="text-2xl font-bold tracking-tight text-foreground">{formHeading}</h2>
                                    )}
                                    {formSubheading && <p className="text-sm text-muted-foreground">{formSubheading}</p>}
                                </div>
                            )}
                            {children}
                        </div>
                    </div>
                </motion.div>
            </div>
        </div>
    );
}

/**
 * Inline field error — the replacement for a toast telling someone their
 * password didn't match three fields and a click away from where they typed
 * it. Always in the same slot under its field, so the layout does not jump
 * when it appears; `role="alert"` so a screen reader announces it without the
 * form having to move focus itself for THIS half of the story (focus
 * management on submit is the caller's job — see signin/signup/
 * update-password — this component only ever renders text).
 */
export function FieldError({ id, children }: { id?: string; children?: ReactNode }) {
    if (!children) return null;
    return (
        <p id={id} role="alert" className="mt-1.5 flex items-start gap-1 text-xs font-medium text-destructive">
            <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            <span>{children}</span>
        </p>
    );
}
