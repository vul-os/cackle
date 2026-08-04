import React from 'react';
import { Link } from 'react-router-dom';
import { Wallet, ShieldCheck, Landmark, FlaskConical } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import Footer from '@/pages/visitor/landing/footer.tsx';
import Header from '@/pages/visitor/header.tsx';
import { BUILD_STATUS_LABEL, NO_FEE_STATEMENT } from '@/components/honesty/claims';

/**
 * What this page used to be, and why none of it is here any more.
 *
 * It advertised a headline drop to a sub-1% rate, a superlative about where
 * that rate sat among competitors, a calculator whose first line item was a
 * percentage cut labelled as ours, card rates for one country's currency and
 * one processor, that country's sales-tax rate, its domestic instant-payment
 * rail, and a single processor named as the product's payment engine. Every
 * one of those was invented. (The exact wording is in the commit that removed
 * it, and deliberately not repeated here — scripts/check-app.mjs scans this
 * file's source text, so quoting a fabrication back would trip the gate that
 * exists to keep it gone.) Checked against the code before rewriting:
 *
 *   - There is no fee anywhere. `grep -rn 'platform_fee|service_fee|our_fee|
 *     application_fee|commission' internal/ cmd/ --include='*.go'` returns
 *     nothing. Cackle has no billing, metering or fee-split code at all, so a
 *     percentage it "charges" could not be collected even in principle.
 *   - There is no privileged processor. `cmd/cackle/main.go` always registers
 *     `manual`; `paystack` only appears if that deployment sets a secret, and
 *     `stub` only under `--demo`. A default binary has no card processor.
 *   - There is no privileged country or currency. Currency is per event, and
 *     the wizard offers the full ISO list.
 *
 * So the honest page is a short one: what Cackle takes (nothing), what a
 * processor may take (its business, not ours, and zero if you use `manual`),
 * and what is not proven yet. No superlative, no rate table, no calculator.
 */

const PricingPage = () => {
    return (
        <div className="flex min-h-screen flex-col bg-background">
            <Header />

            <main className="flex-grow pt-24">
                {/* On a `--primary` fill the only ink permitted by the palette
                    rules is `--primary-foreground` (INK, 6.9:1). This band
                    used `text-white` at 3.34:1 and `text-white/70` at ~2:1 —
                    both below AA for the sizes involved. */}
                <div className="bg-primary">
                    <div className="mx-auto max-w-5xl px-4 py-4">
                        <p className="text-center text-lg font-semibold text-primary-foreground">
                            Cackle takes 0% of what you sell.
                        </p>
                    </div>
                </div>

                <div className="mx-auto max-w-5xl px-4 py-16">
                    <div className="mb-16 text-center">
                        <span className="mb-4 block text-sm font-semibold uppercase tracking-wider text-primary-emphasis">
                            What this costs
                        </span>
                        <h1 className="mb-6 font-display text-4xl font-bold tracking-tight sm:text-5xl">
                            Cackle has no fees
                        </h1>
                        <p className="mx-auto max-w-2xl text-lg text-muted-foreground">
                            {NO_FEE_STATEMENT} You run it yourself. The only money question left is what your own
                            payment provider charges you — and that is your arrangement with them, not ours.
                        </p>
                    </div>

                    <div className="mb-8 grid gap-8 md:grid-cols-2">
                        <Card>
                            <CardContent className="p-8">
                                <div className="mb-3 flex items-center gap-2 text-primary-emphasis">
                                    <Wallet className="h-5 w-5" aria-hidden="true" />
                                    <h2 className="text-xl font-bold text-foreground">What Cackle charges</h2>
                                </div>
                                <p className="mb-4 text-3xl font-bold">Nothing</p>
                                <p className="leading-relaxed text-muted-foreground">
                                    No per-ticket cut, no percentage, no monthly plan, no seat count. There is no
                                    billing code in this software to charge you with. It is a program you run on your
                                    own machine, and it stays out of the money entirely.
                                </p>
                            </CardContent>
                        </Card>

                        <Card>
                            <CardContent className="p-8">
                                <div className="mb-3 flex items-center gap-2 text-primary-emphasis">
                                    <Landmark className="h-5 w-5" aria-hidden="true" />
                                    <h2 className="text-xl font-bold text-foreground">
                                        What your payment provider charges
                                    </h2>
                                </div>
                                <p className="mb-4 text-3xl font-bold">Ask them</p>
                                <p className="leading-relaxed text-muted-foreground">
                                    Whatever you have agreed with whoever moves the money. It varies by processor, by
                                    country and by currency, and it can change without Cackle knowing. Cackle does not
                                    set that rate, does not see it, and does not add anything on top of it — so no
                                    honest number can be printed on this page.
                                </p>
                            </CardContent>
                        </Card>
                    </div>

                    <Card className="mb-8">
                        <CardContent className="p-8">
                            <h2 className="mb-3 text-xl font-bold">
                                Out of the box there is no processor at all
                            </h2>
                            <p className="mb-4 leading-relaxed text-muted-foreground">
                                Cackle&apos;s default way of taking payment is called <code className="rounded bg-muted px-1.5 py-0.5 text-[0.9375rem] font-semibold">manual</code>:
                                the money reaches you however it already does — bank transfer, cash at the door, an
                                invoice, mobile money — and you mark the order paid. No payment account to open, no API
                                key, no country requirement, and nobody takes a percentage, because no processor is in
                                the loop.
                            </p>
                            <p className="leading-relaxed text-muted-foreground">
                                If you would rather take cards or crypto, optional adapters plug in per deployment. No
                                one of them is the default and none is bundled with the product&apos;s promises — which
                                processors exist, and which are actually proven safe with real money today, is written
                                down in <Link to="/docs" className="font-semibold text-primary-emphasis underline underline-offset-2">the docs</Link>.
                            </p>
                        </CardContent>
                    </Card>

                    <Card>
                        <CardContent className="grid gap-8 p-8 md:grid-cols-2 md:gap-12">
                            <div>
                                <div className="mb-3 flex items-center gap-2 text-primary-emphasis">
                                    <ShieldCheck className="h-5 w-5" aria-hidden="true" />
                                    <h2 className="text-xl font-bold text-foreground">Cackle never holds your funds</h2>
                                </div>
                                <p className="leading-relaxed text-muted-foreground">
                                    Money goes from the buyer to your payment provider to you, or straight to you under{' '}
                                    <code className="rounded bg-muted px-1.5 py-0.5 text-[0.9375rem] font-semibold">manual</code>. Cackle
                                    keeps the record of what was sold and admitted. It is never in the custody chain,
                                    which is also why it can never take a cut.
                                </p>
                            </div>
                            <div className="md:border-l md:border-border md:pl-12">
                                <div className="mb-3 flex items-center gap-2 text-primary-emphasis">
                                    <FlaskConical className="h-5 w-5" aria-hidden="true" />
                                    <h2 className="text-xl font-bold text-foreground">{BUILD_STATUS_LABEL}</h2>
                                </div>
                                <p className="leading-relaxed text-muted-foreground">
                                    Payment adapters are unit-tested, not sandbox-verified, unless their documentation
                                    says otherwise. Do not take real money through an adapter that has not been proven
                                    against the real processor.
                                </p>
                            </div>
                        </CardContent>
                    </Card>
                </div>
            </main>

            <Footer />
        </div>
    );
};

export default PricingPage;
