import React from 'react';
import { Link } from 'react-router-dom';
import Header from '@/pages/visitor/header';
import Footer from '@/pages/visitor/landing/footer';
import { Separator } from '@/components/ui/separator';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { AlertTriangle, WifiOff, QrCode, ShieldCheck, ArrowRight, ExternalLink } from 'lucide-react';
import { TAP_LINK } from '@/pages/visitor/ui-scale';

// Layman first, engineer second, on the same page. The person reading this
// runs a music venue, a conference, a church hall or a night market — so the
// top of the page says what happens at the door, and the signing scheme
// waits until they have a reason to care.
//
// Two things this file previously got WRONG, and must not get wrong again:
//
//   1. It named Paystack as though card payment were the way Cackle takes
//      money. It is one optional rail of several, it is not sandbox-verified
//      here, and the rail that is always on and cannot be switched off is
//      `manual` — cash, transfer, paid-at-the-door — which makes no network
//      call at all.
//
//   2. It omitted the cross-gate double-scan limitation entirely. That is
//      not an oversight to be corrected quietly: a venue staffs its doors
//      around this fact, and a page that leaves it out is a page that costs
//      somebody a real evening. It is stated below, in the same words the
//      marketing site uses, and it does not get softened.

interface DocsSection {
    id: string;
    label: string;
}

const SECTIONS: DocsSection[] = [
    { id: 'what', label: 'What it does' },
    { id: 'attendees', label: 'If you bought a ticket' },
    { id: 'organisers', label: 'If you are running the event' },
    { id: 'limits', label: 'What it cannot do' },
    { id: 'money', label: 'Getting paid' },
    { id: 'under-the-hood', label: 'How it actually works' },
    { id: 'self-host', label: 'Running it yourself' },
];

interface StepProps {
    n: string;
    title: string;
    children: React.ReactNode;
}

const Step = ({ n, title, children }: StepProps) => (
    <li className="relative flex gap-4">
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary text-sm font-black text-primary-foreground">
            {n}
        </span>
        <div className="min-w-0 pb-8">
            <h3 className="text-base font-bold tracking-tight text-foreground">{title}</h3>
            <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{children}</p>
        </div>
    </li>
);

interface SectionProps {
    id: string;
    title: string;
    kicker?: string;
    children: React.ReactNode;
}

const Section = ({ id, title, kicker, children }: SectionProps) => (
    <section id={id} className="scroll-mt-24">
        {kicker && <span className="text-xs font-bold uppercase tracking-[0.16em] text-primary-emphasis">{kicker}</span>}
        <h2 className="mt-2 font-display text-display-sm font-bold tracking-tight sm:text-display-md">{title}</h2>
        <div className="mt-5 space-y-4 text-base leading-relaxed text-muted-foreground">{children}</div>
    </section>
);

const DocsPage = () => (
    <div className="flex min-h-screen flex-col bg-background">
        <Header />
        <main id="main" className="flex-1 pt-16">
            {/* Hero */}
            <div className="border-b border-border bg-muted/40">
                <div className="mx-auto max-w-5xl px-4 py-12 sm:py-16">
                    <Badge variant="accent" className="mb-4">
                        <AlertTriangle className="h-3 w-3" aria-hidden="true" />
                        Experimental — not production-ready
                    </Badge>
                    <h1 className="max-w-2xl font-display text-display-md font-extrabold tracking-tight sm:text-display-lg">
                        How Cackle works
                    </h1>
                    <p className="mt-4 max-w-2xl text-base leading-relaxed text-muted-foreground sm:text-lg">
                        Sell tickets online. Check them at the door on an ordinary phone that keeps working when the
                        venue&apos;s internet drops. That is the whole product — everything below is detail.
                    </p>
                </div>
            </div>

            <div className="mx-auto max-w-5xl px-4 py-10 sm:py-14">
                <div className="lg:grid lg:grid-cols-[200px_1fr] lg:gap-12">
                    {/* On this page */}
                    <nav aria-label="On this page" className="mb-10 lg:mb-0">
                        <div className="lg:sticky lg:top-24">
                            <p className="text-xs font-bold uppercase tracking-[0.14em] text-muted-foreground">On this page</p>
                            <ul className="mt-3 space-y-0.5 lg:space-y-1">
                                {SECTIONS.map((s) => (
                                    <li key={s.id}>
                                        <a
                                            href={`#${s.id}`}
                                            className={`${TAP_LINK} w-full text-sm text-foreground/75 transition-colors hover:text-primary-emphasis lg:min-h-0 lg:py-1`}
                                        >
                                            {s.label}
                                        </a>
                                    </li>
                                ))}
                            </ul>
                        </div>
                    </nav>

                    <div className="min-w-0 space-y-14">
                        <Section id="what" kicker="The idea" title="Your door works with no internet">
                            <p>
                                A normal ticketing system checks every ticket against a server somewhere. When the wifi at
                                the venue drops, the queue stops. Cackle puts the answer inside the ticket itself, so the
                                phone at the door can check it without asking anybody.
                            </p>
                            <p>
                                Staff open the scanner once while they still have signal. After that they can turn the
                                network off entirely and keep letting people in all night. When the phone gets a connection
                                again, it catches up on its own.
                            </p>
                        </Section>

                        <Separator variant="perforated" />

                        <Section id="attendees" kicker="For attendees" title="If you bought a ticket">
                            <ol className="not-prose mt-2 list-none space-y-0 p-0">
                                <Step n="1" title="Buy it">
                                    Pick your tickets, pay however the organiser takes money, and the order is yours. Every
                                    organiser sets their own prices, in their own currency.
                                </Step>
                                <Step n="2" title="Find it under My tickets">
                                    Each ticket has a QR code. Screenshot it, print it, or just open the page. It does not
                                    expire and it does not need to phone home.
                                </Step>
                                <Step n="3" title="Show it at the door">
                                    Hold up the QR. It works whether or not the venue&apos;s wifi is up, and whether or not
                                    your own phone has signal — as long as you can get the picture on screen.
                                </Step>
                            </ol>
                            <div className="flex flex-wrap gap-3 pt-1">
                                <Button asChild>
                                    <Link to="/tickets">
                                        My tickets
                                        <ArrowRight className="ml-2 h-4 w-4" aria-hidden="true" />
                                    </Link>
                                </Button>
                                <Button variant="outline" asChild>
                                    <Link to="/events">What&apos;s on</Link>
                                </Button>
                            </div>
                        </Section>

                        <Separator variant="perforated" />

                        <Section id="organisers" kicker="For organisers" title="If you are running the event">
                            <ol className="not-prose mt-2 list-none space-y-0 p-0">
                                <Step n="1" title="Make the event">
                                    Add your ticket types and prices, then publish. Nothing is on sale until you say so.
                                </Step>
                                <Step n="2" title="Open the scanner once, while online">
                                    That single step downloads everything the door needs for the whole event. Do it before
                                    doors, on the venue&apos;s wifi or on your own data — it takes seconds.
                                </Step>
                                <Step n="3" title="Unplug and scan all night">
                                    Fake and already-used tickets are caught right there on the phone. Nothing is sent
                                    anywhere while you scan.
                                </Step>
                                <Step n="4" title="Reconnect when it suits you">
                                    Every scan the phone recorded syncs back, and you get the real admission list.
                                </Step>
                            </ol>
                        </Section>

                        <Separator variant="perforated" />

                        {/* The limitation. It is a full section with its own
                            visual weight, not a footnote, because a venue
                            staffs its doors around it. */}
                        <Section id="limits" kicker="Read this one" title="What it cannot do">
                            <div className="not-prose rounded-xl border border-destructive/30 bg-destructive/5 p-5 sm:p-6">
                                <div className="flex items-start gap-3">
                                    <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-destructive" aria-hidden="true" />
                                    <div className="min-w-0">
                                        <h3 className="text-base font-bold tracking-tight text-foreground">
                                            Two offline doors cannot stop the same ticket being used twice
                                        </h3>
                                        <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                                            If somebody tries the same ticket at your north door and your south door while
                                            both are offline, <strong className="font-semibold text-foreground">both gates
                                            will open</strong>. Cackle shows you that it happened once the phones
                                            reconnect — <strong className="font-semibold text-foreground">it cannot stop it
                                            at the second door</strong>. Cross-gate double-scan is detected, not prevented.
                                        </p>
                                        <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
                                            Two doors that are offline cannot talk to each other at the moment of a scan, so
                                            no amount of clever syncing changes this, and nothing in Cackle claims
                                            otherwise. One phone on its own catches its own repeats every time. If a
                                            duplicate at a second door would be expensive for you, run one door, or keep the
                                            doors online.
                                        </p>
                                    </div>
                                </div>
                            </div>

                            <ul className="not-prose mt-5 space-y-3 text-sm leading-relaxed text-muted-foreground">
                                <li className="flex gap-3">
                                    <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-primary" aria-hidden="true" />
                                    <span>
                                        <strong className="font-semibold text-foreground">Cackle is experimental.</strong> It
                                        is built and tested, and it has not been run at scale by anybody. Treat a first
                                        event as a trial.
                                    </span>
                                </li>
                                <li className="flex gap-3">
                                    <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-primary" aria-hidden="true" />
                                    <span>
                                        Ticket transfers and resale, on-site closed-loop payments, and delivery straight to
                                        an inbox are <strong className="font-semibold text-foreground">not built yet</strong>.
                                    </span>
                                </li>
                                <li className="flex gap-3">
                                    <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-primary" aria-hidden="true" />
                                    <span>
                                        Syncing between phones moves door records only. It is not a backup, and it does not
                                        prevent a double admission either.
                                    </span>
                                </li>
                            </ul>
                        </Section>

                        <Separator variant="perforated" />

                        <Section id="money" kicker="Money" title="Getting paid">
                            <p>
                                Cackle never holds your money. You connect your own way of getting paid, and the funds go
                                where they were always going.
                            </p>
                            <div className="not-prose grid gap-4 sm:grid-cols-2">
                                <div className="rounded-xl border border-border bg-card p-5">
                                    <div className="flex flex-wrap items-center gap-2">
                                        <h3 className="text-base font-bold tracking-tight text-foreground">Cash, transfer, at the door</h3>
                                        <Badge variant="success">Always on</Badge>
                                    </div>
                                    <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                                        Built into every copy and impossible to switch off. No keys, no accounts, no network
                                        call — they pay you however you already get paid and you mark the order settled.
                                        This on its own is a complete way to run an event.
                                    </p>
                                </div>
                                <div className="rounded-xl border border-border bg-card p-5">
                                    <div className="flex flex-wrap items-center gap-2">
                                        <h3 className="text-base font-bold tracking-tight text-foreground">Card and other processors</h3>
                                        <Badge variant="warning">Not sandbox-verified</Badge>
                                    </div>
                                    <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                                        Cackle is not tied to one processor or one country, and adds none of its own fees.
                                        The card and crypto rails are written and unit-tested but have not been run against
                                        a live processor here.{' '}
                                        <strong className="font-semibold text-foreground">
                                            Do not take real money through one until you have tested it yourself.
                                        </strong>
                                    </p>
                                </div>
                            </div>
                        </Section>

                        <Separator variant="perforated" />

                        {/* The engineer's half of the page. It lives below the
                            plain-language half rather than instead of it. */}
                        <Section id="under-the-hood" kicker="For the technical reader" title="How it actually works">
                            <div className="not-prose grid gap-4 sm:grid-cols-3">
                                {[
                                    {
                                        icon: QrCode,
                                        title: 'The ticket is the proof',
                                        body: 'A ticket is a compact Ed25519-signed capability, small enough to fit in a QR code. It carries its own claims rather than pointing at a row in a database.',
                                    },
                                    {
                                        icon: ShieldCheck,
                                        title: 'The gate verifies locally',
                                        body: "The scanner checks the signature against an event key pinned before doors. A forged or edited ticket fails on the phone, with nothing to ask.",
                                    },
                                    {
                                        icon: WifiOff,
                                        title: 'The network is never in the path',
                                        body: 'Admission does not wait on a request. Scans are stored verbatim on the device and reconciled later, which is why the limitation above is a limitation and not a bug.',
                                    },
                                ].map(({ icon: Icon, title, body }) => (
                                    <div key={title} className="rounded-xl border border-border bg-card p-5">
                                        <Icon className="h-6 w-6 text-primary-emphasis" aria-hidden="true" />
                                        <h3 className="mt-3 text-sm font-bold tracking-tight text-foreground">{title}</h3>
                                        <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{body}</p>
                                    </div>
                                ))}
                            </div>
                        </Section>

                        <Separator variant="perforated" />

                        <Section id="self-host" kicker="Self-hosting" title="Running it yourself">
                            <p>
                                Cackle is one Go binary with the database and this whole web interface inside it. There is
                                no cluster to stand up and nothing to point it at.
                            </p>
                            <pre className="not-prose overflow-x-auto rounded-xl border border-border bg-muted/60 p-4 text-sm">
                                <code className="font-mono">docker run -p 8080:8080 vulos/cackle</code>
                            </pre>
                            <p className="text-sm">
                                Configuration, the API and the ticket format are documented in the repository.
                            </p>
                            <div className="pt-1">
                                <Button variant="outline" asChild>
                                    <a href="https://github.com/vul-os/cackle" target="_blank" rel="noopener noreferrer">
                                        Read the source
                                        <ExternalLink className="ml-2 h-3.5 w-3.5 opacity-70" aria-hidden="true" />
                                    </a>
                                </Button>
                            </div>
                        </Section>
                    </div>
                </div>
            </div>
        </main>
        <Footer />
    </div>
);

export default DocsPage;
