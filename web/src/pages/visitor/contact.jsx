import React from 'react';
import { Link } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { BookOpen, Github, Ticket, Building2, ArrowRight, ExternalLink } from 'lucide-react';
import Footer from '@/pages/visitor/landing/footer';
import Header from '@/pages/visitor/header';

// ── Why there is no contact form on this page ─────────────────────────────
//
// There used to be one. Filling it in and pressing Send fired ZERO network
// requests and then told you "We've received your message and will reply
// within 1-2 business days." Nobody received anything: there is no contact
// endpoint in the API and no inbox behind it. The page also printed a phone
// number and an email address that do not exist.
//
// A form that swallows a refund request and answers "we'll get back to you"
// is worse than no form, because it stops the person asking somebody who
// can actually help. So the page now answers the real question — WHO do I
// ask — and every route on it goes somewhere that exists.
//
// The honest answer is also the useful one: Cackle is software an organiser
// runs themselves. This deployment cannot know its operator's support
// address, and it must not invent one.

const REPO_ISSUES = 'https://github.com/vul-os/cackle/issues';

const Route = ({ icon: Icon, title, children, action }) => (
    <div className="flex gap-4 py-6">
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-accent text-accent-foreground">
            <Icon className="h-5 w-5" aria-hidden="true" />
        </div>
        <div className="min-w-0 grow">
            <h3 className="text-base font-bold tracking-tight text-foreground">{title}</h3>
            <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{children}</p>
            {action && <div className="mt-3">{action}</div>}
        </div>
    </div>
);

const ContactPage = () => (
    <div className="flex min-h-screen flex-col bg-background">
        <Header />
        <main id="main" className="flex-1 pt-16">
            <section className="mx-auto max-w-3xl px-4 py-12 sm:py-16">
                <div className="max-w-2xl">
                    <span className="text-xs font-bold uppercase tracking-[0.16em] text-primary-emphasis">Get help</span>
                    <h1 className="mt-3 font-display text-display-md font-extrabold tracking-tight sm:text-display-lg">
                        Who to ask
                    </h1>
                    <p className="mt-4 text-base leading-relaxed text-muted-foreground">
                        Cackle is software that whoever put this event on runs themselves. So the right person to ask
                        depends on what you need — here is the whole list, shortest route first.
                    </p>
                </div>

                <Separator variant="perforated" className="my-10" />

                <Card>
                    <CardContent className="divide-y divide-border p-5 sm:p-6">
                        <Route
                            icon={Ticket}
                            title="A ticket you bought, a refund, or getting in on the night"
                            action={
                                <Button variant="outline" asChild>
                                    <Link to="/orders">
                                        Find my order
                                        <ArrowRight className="ml-2 h-4 w-4" aria-hidden="true" />
                                    </Link>
                                </Button>
                            }
                        >
                            Ask the organiser who sold it to you — they hold the money, the guest list and the door. Their
                            details are on the event page and on your order. Cackle never holds funds, so nobody here can
                            issue you a refund.
                        </Route>

                        <Route
                            icon={Building2}
                            title="You want to sell tickets with Cackle"
                            action={
                                <Button variant="outline" asChild>
                                    <Link to="/pricing">
                                        See what it costs
                                        <ArrowRight className="ml-2 h-4 w-4" aria-hidden="true" />
                                    </Link>
                                </Button>
                            }
                        >
                            You run it yourself: one file, your own machine, your own payment account. Start with the
                            pricing page, then the docs. Cackle is experimental — try it on a small night first.
                        </Route>

                        <Route
                            icon={BookOpen}
                            title="How something works, or what it cannot do"
                            action={
                                <Button variant="outline" asChild>
                                    <Link to="/docs">
                                        Read the docs
                                        <ArrowRight className="ml-2 h-4 w-4" aria-hidden="true" />
                                    </Link>
                                </Button>
                            }
                        >
                            The docs say plainly what is built, what is not, and the one thing that can never work. Start
                            there before anything else.
                        </Route>

                        <Route
                            icon={Github}
                            title="A bug, or a question about the software itself"
                            action={
                                <Button variant="outline" asChild>
                                    <a href={REPO_ISSUES} target="_blank" rel="noopener noreferrer">
                                        Open an issue
                                        <ExternalLink className="ml-2 h-3.5 w-3.5 opacity-70" aria-hidden="true" />
                                    </a>
                                </Button>
                            }
                        >
                            Cackle is open source, MIT licensed. Issues are public and are read by the people who build it.
                            This is the only channel the project itself has.
                        </Route>
                    </CardContent>
                </Card>

                <p className="mt-8 rounded-xl border border-border bg-muted/40 px-4 py-3 text-sm leading-relaxed text-muted-foreground">
                    <span className="font-semibold text-foreground">There is no contact form on this page on purpose.</span>{' '}
                    This copy of Cackle has no inbox to send one to, and a form that quietly discards a refund request while
                    saying &ldquo;we&apos;ll get back to you&rdquo; would be worse than no form at all.
                </p>
            </section>
        </main>
        <Footer />
    </div>
);

export default ContactPage;
