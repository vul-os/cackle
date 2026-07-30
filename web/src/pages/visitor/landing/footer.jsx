import React from 'react';
import { Link } from 'react-router-dom';
import { BrandLockup } from '@/components/brand/wordmark';
import { Separator } from '@/components/ui/separator';

const PLATFORM = [
    { to: '/events', label: 'Browse events' },
    { to: '/pricing', label: 'Sell tickets' },
    { to: '/docs', label: 'How it works' },
    { to: '/contact', label: 'Contact' },
];

const ACCOUNT = [
    { to: '/login', label: 'Log in' },
    { to: '/orders', label: 'My orders' },
    { to: '/tickets', label: 'My tickets' },
];

const FooterColumn = ({ title, links }) => (
    <div>
        <h2 className="text-xs font-bold uppercase tracking-[0.14em] text-muted-foreground">{title}</h2>
        <ul className="mt-4 space-y-3 text-sm">
            {links.map(({ to, label }) => (
                <li key={to}>
                    <Link to={to} className="text-foreground/80 transition-colors hover:text-primary-emphasis">
                        {label}
                    </Link>
                </li>
            ))}
        </ul>
    </div>
);

const Footer = () => (
    <footer className="mt-auto bg-muted/50">
        {/* The page tears off here. This is the one seam on a marketing page
            where the stub motif is literal rather than ornamental — the
            content above is the ticket, the footer below is the counterfoil.
            --notch punches through to the page ground the rule sits on. */}
        <Separator variant="perforated" className="mx-auto max-w-[calc(100%-2rem)]" style={{ '--notch': 'var(--background)' }} />

        <div className="container mx-auto px-4 py-12 sm:py-14">
            <div className="grid grid-cols-2 gap-x-8 gap-y-10 md:grid-cols-4">
                <div className="col-span-2">
                    <Link to="/" className="inline-flex rounded-md" aria-label="Cackle — home">
                        <BrandLockup size="md" />
                    </Link>
                    <p className="mt-4 max-w-sm text-sm leading-relaxed text-muted-foreground">
                        Sell tickets online, then check them at the door with a phone that keeps working when the venue&apos;s
                        internet drops.
                    </p>
                    {/* States a limitation, so it stays — in these words or
                        stronger ones. A footer that quietly drops it is a
                        page claiming more than the product does. */}
                    <p className="mt-4 max-w-sm rounded-lg border border-border bg-background/60 px-3 py-2 text-xs leading-relaxed text-muted-foreground">
                        <span className="font-semibold text-foreground">Cackle is experimental and not production-ready.</span>{' '}
                        Try it on a small night before you put a sold-out one through it.
                    </p>
                </div>

                <FooterColumn title="Platform" links={PLATFORM} />
                <FooterColumn title="Account" links={ACCOUNT} />
            </div>

            <div className="mt-12 flex flex-col gap-3 border-t border-border pt-6 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
                <p>© {new Date().getFullYear()} Cackle. MIT licensed.</p>
                <p>
                    Part of{' '}
                    <a
                        href="https://vulos.org"
                        target="_blank"
                        rel="noopener noreferrer"
                        className="font-medium text-primary-emphasis hover:underline"
                    >
                        VulOS
                    </a>{' '}
                    — runs standalone, or hosted by the Vulos OS.
                </p>
            </div>
        </div>
    </footer>
);

export default Footer;
