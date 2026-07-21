import React, { useState } from 'react';
import { motion } from 'framer-motion';
import { Search, WifiOff, Globe2, ShieldCheck } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import Logo from '/cackle.svg';

const SIGNALS = [
    { icon: WifiOff, label: 'Offline-verified gate' },
    { icon: Globe2, label: 'Any country, any currency' },
    { icon: ShieldCheck, label: 'Cackle never holds funds' },
];

function Hero({ query, onSearch }) {
    const [value, setValue] = useState(query);

    const handleSubmit = (e) => {
        e.preventDefault();
        onSearch(value);
    };

    return (
        <div className="relative overflow-hidden bg-zinc-950 pb-16 pt-32 sm:pb-24 sm:pt-40">
            {/* Atmosphere: brand glow + dot-grid texture + an oversized watermark
                of the ticket mark itself, bled off the right edge for asymmetry —
                identity, not decoration duplicated from the logo. */}
            <div className="pointer-events-none absolute inset-0">
                <div className="absolute -top-1/3 left-1/2 h-[60rem] w-[60rem] -translate-x-1/2 rounded-full bg-primary/25 blur-[140px]" />
                <div className="dot-grid absolute inset-0 opacity-40" />
                <img
                    src={Logo}
                    alt=""
                    aria-hidden="true"
                    className="absolute -right-24 -top-16 hidden h-[34rem] w-[34rem] rotate-[14deg] opacity-[0.07] mix-blend-screen lg:block"
                />
            </div>

            <div className="container relative mx-auto px-4">
                <div className="mx-auto max-w-3xl text-center">
                    <motion.div
                        initial={{ opacity: 0, y: 12 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.5 }}
                        className="mb-5 flex flex-wrap items-center justify-center gap-2"
                    >
                        {SIGNALS.map(({ icon: Icon, label }) => (
                            <span
                                key={label}
                                className="inline-flex items-center gap-1.5 rounded-full border border-white/15 bg-white/5 px-3.5 py-1.5 text-xs font-semibold uppercase tracking-wide text-white/70 backdrop-blur-sm"
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
                        className="mx-auto max-w-3xl font-display text-display-xl font-black tracking-tight text-white sm:text-display-2xl"
                    >
                        Your gate works<br className="hidden sm:block" /> with no internet.
                    </motion.h1>
                    <motion.p
                        initial={{ opacity: 0, y: 16 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.5, delay: 0.1 }}
                        className="mx-auto mt-6 max-w-xl text-lg text-white/70"
                    >
                        Every Cackle ticket is a signed, offline-verifiable capability. The venue is never in the
                        critical path of admission — scan tickets on a phone, with the network unplugged.
                    </motion.p>

                    <motion.form
                        initial={{ opacity: 0, y: 16 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.5, delay: 0.15 }}
                        onSubmit={handleSubmit}
                        className="mx-auto mt-10 flex max-w-xl gap-2 rounded-2xl border border-white/10 bg-white/10 p-2 shadow-[0_20px_60px_-20px_rgba(0,0,0,0.6)] backdrop-blur"
                    >
                        <div className="relative flex-1">
                            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-white/50" />
                            <Input
                                value={value}
                                onChange={(e) => setValue(e.target.value)}
                                placeholder="Search events, venues, or organisers"
                                className="border-0 bg-transparent pl-10 text-white placeholder:text-white/50 focus-visible:ring-1 focus-visible:ring-white/30"
                            />
                        </div>
                        <Button type="submit" size="lg">
                            Search
                        </Button>
                    </motion.form>
                </div>
            </div>

            {/* Tear seam into the category strip below — .ticket-tear defaults
                to var(--background), which is exactly what sits underneath. */}
            <div className="relative mt-16 sm:mt-20">
                <div className="ticket-tear" aria-hidden="true" />
            </div>
        </div>
    );
}

export default Hero;
