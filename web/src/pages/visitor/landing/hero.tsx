import React, { useState } from 'react';
import { motion } from 'framer-motion';
import { Search, WifiOff, Globe2, ShieldCheck } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { LogoTile } from '@/components/brand/wordmark';

const SIGNALS = [
    { icon: WifiOff, label: 'The door works offline' },
    { icon: Globe2, label: 'Any country, any currency' },
    { icon: ShieldCheck, label: 'Cackle never holds funds' },
];

export interface HeroProps {
    query: string;
    onSearch: (value: string) => void;
}

function Hero({ query, onSearch }: HeroProps) {
    const [value, setValue] = useState(query);

    const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        onSearch(value);
    };

    return (
        // INK, the brand's third colour, rather than a near-black borrowed
        // from a utility scale: on this page the dark band IS a brand
        // surface, and it has to be the same ink the dark theme grounds on.
        // `bg-brand-2` / `text-brand-ink` are the FIXED-identity tokens
        // (tailwind.config.js), not raw `white`/`black` — the band stays the
        // same ink and the type on it stays the mark's own white regardless
        // of which theme the rest of the page is in, which is exactly what a
        // fixed brand surface means. See index.css's note on `--brand-ink`.
        //
        // This is a deliberate, committed brand surface, not a default: full
        // bleed, no cap on how tall it's allowed to be, and all of its
        // bottom padding lives on the CONTENT wrapper below rather than on
        // this outer element. That matters because the outer element ends
        // the moment the tear line does — put padding out here as well and
        // the ink keeps going a whole extra section-height past the visible
        // "tear", which is exactly the dead ink strip this used to leave
        // sitting above the category row.
        <div className="relative overflow-hidden bg-brand-2 pt-24 sm:pt-32 lg:pt-40">
            {/* Atmosphere: brand glow + dot-grid texture + an oversized
                watermark of the ticket mark itself, bled off the right edge
                for asymmetry — identity, not decoration invented alongside
                the logo. Drawn from the shared <LogoTile> so there is exactly
                one import of the mark in the whole app. */}
            <div className="pointer-events-none absolute inset-0">
                <div className="absolute -top-1/3 left-1/2 h-[60rem] w-[60rem] -translate-x-1/2 rounded-full bg-primary/25 blur-[140px]" />
                <div className="dot-grid absolute inset-0 opacity-40" />
                <LogoTile
                    aria-hidden="true"
                    className="absolute -right-28 -top-20 hidden h-[38rem] w-[38rem] rotate-[14deg] opacity-[0.08] mix-blend-screen lg:block"
                />
            </div>

            <div className="container relative mx-auto px-4 pb-24 sm:pb-32 lg:pb-40">
                <div className="mx-auto max-w-3xl text-center">
                    <motion.div
                        initial={{ opacity: 0, y: 12 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.5 }}
                        className="mb-6 flex flex-wrap items-center justify-center gap-2"
                    >
                        {SIGNALS.map(({ icon: Icon, label }) => (
                            <span
                                key={label}
                                className="inline-flex items-center gap-1.5 rounded-full border border-brand-ink/15 bg-brand-ink/5 px-3.5 py-1.5 text-xs font-semibold uppercase tracking-wide text-brand-ink/75 backdrop-blur-sm"
                            >
                                <Icon className="h-3.5 w-3.5 text-primary" aria-hidden="true" />
                                {label}
                            </span>
                        ))}
                    </motion.div>
                    <motion.h1
                        initial={{ opacity: 0, y: 16 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.5, delay: 0.05 }}
                        // Steps down two display sizes on a phone. At
                        // display-xl the second line overhangs a 390px
                        // viewport, and the promise this page makes is zero
                        // horizontal overflow.
                        className="mx-auto max-w-3xl text-balance font-display text-display-lg font-black tracking-tight text-brand-ink sm:text-display-xl lg:text-display-2xl"
                    >
                        Your door works
                        <br className="hidden sm:block" /> with no internet.
                    </motion.h1>
                    <motion.p
                        initial={{ opacity: 0, y: 16 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.5, delay: 0.1 }}
                        className="mx-auto mt-7 max-w-xl text-base leading-relaxed text-brand-ink/75 sm:text-lg"
                    >
                        Sell tickets online. Then check them at the door on an ordinary phone — no wifi, no signal, no
                        server in the building. It remembers who came in and catches up when the internet comes back.
                    </motion.p>

                    <motion.form
                        initial={{ opacity: 0, y: 16 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.5, delay: 0.15 }}
                        onSubmit={handleSubmit}
                        role="search"
                        className="mx-auto mt-12 flex max-w-xl gap-2 rounded-2xl border border-brand-ink/10 bg-brand-ink/10 p-2 shadow-floating backdrop-blur"
                    >
                        <div className="relative flex-1">
                            <Search
                                className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-brand-ink/50"
                                aria-hidden="true"
                            />
                            {/* Scoped to this host's own events — the search
                                reaches nothing else, so it must not offer to. */}
                            <label htmlFor="hero-search" className="sr-only">
                                Search these events
                            </label>
                            <Input
                                id="hero-search"
                                value={value}
                                onChange={(e) => setValue(e.target.value)}
                                placeholder="Search these events"
                                className="border-0 bg-transparent pl-10 text-brand-ink placeholder:text-brand-ink/50 focus-visible:ring-1 focus-visible:ring-brand-ink/40"
                            />
                        </div>
                        <Button type="submit" size="lg" className="shrink-0 px-5 sm:px-8">
                            Search
                        </Button>
                    </motion.form>
                </div>
            </div>

            {/* Tear seam into the category strip below, flush against this
                band's own bottom edge — .ticket-tear defaults to
                var(--background), which is exactly what sits underneath. No
                padding follows it: this element ending IS the hero ending. */}
            <div className="ticket-tear" aria-hidden="true" />
        </div>
    );
}

export default Hero;
