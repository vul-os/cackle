import React, { useState } from 'react';
import { motion } from 'framer-motion';
import { Search } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';

function Hero({ query, onSearch }) {
    const [value, setValue] = useState(query);

    const handleSubmit = (e) => {
        e.preventDefault();
        onSearch(value);
    };

    return (
        <div className="relative overflow-hidden bg-zinc-950 pb-24 pt-32 sm:pb-32 sm:pt-40">
            <div className="pointer-events-none absolute inset-0">
                <div className="absolute -top-1/3 left-1/2 h-[60rem] w-[60rem] -translate-x-1/2 rounded-full bg-primary/25 blur-[140px]" />
            </div>

            <div className="container relative mx-auto px-4 text-center">
                <motion.p
                    initial={{ opacity: 0, y: 12 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ duration: 0.5 }}
                    className="mb-4 text-sm font-semibold uppercase tracking-[0.2em] text-primary"
                >
                    Events &amp; ticketing
                </motion.p>
                <motion.h1
                    initial={{ opacity: 0, y: 16 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ duration: 0.5, delay: 0.05 }}
                    className="mx-auto max-w-3xl font-display text-4xl font-black tracking-tight text-white sm:text-6xl"
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
                    className="mx-auto mt-10 flex max-w-xl gap-2 rounded-2xl bg-white/10 p-2 backdrop-blur"
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
    );
}

export default Hero;
