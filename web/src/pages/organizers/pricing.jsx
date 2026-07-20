import React from 'react';
import { CreditCard, ArrowRight, ShieldCheck, Clock3 } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import Footer from '@/pages/visitor/landing/footer.jsx';
import Header from '@/pages/visitor/header.jsx';
import PaymentCalculator from './payment-calculator';

const RATES = [
    { name: 'Local cards', description: 'South African-issued cards', fee: '2.9% + R1.00' },
    { name: 'International cards', description: 'Non-South African cards', fee: '3.9% + R1.00' },
];

const PricingPage = () => {
    return (
        <div className="flex min-h-screen flex-col bg-background">
            <Header />

            <main className="flex-grow pt-24">
                <div className="bg-gradient-to-r from-primary to-primary/80">
                    <div className="mx-auto max-w-5xl px-4 py-4">
                        <div className="flex flex-wrap items-center justify-center gap-4 text-center">
                            <div className="flex items-center">
                                <span className="text-2xl font-bold text-white/70 line-through">2%</span>
                                <ArrowRight className="mx-3 h-6 w-6 text-white" />
                                <span className="text-3xl font-bold text-white">0.85%</span>
                            </div>
                            <div className="hidden h-8 w-px bg-white/30 sm:block" />
                            <p className="text-lg font-medium text-white">The lowest fees in the market</p>
                        </div>
                    </div>
                </div>

                <div className="mx-auto max-w-5xl px-4 py-16">
                    <div className="mb-16 text-center">
                        <span className="mb-4 block text-sm font-semibold uppercase tracking-wider text-primary">Payment Solutions</span>
                        <h1 className="mb-6 font-display text-4xl font-bold tracking-tight sm:text-5xl">Transparent Pricing</h1>
                        <p className="mx-auto max-w-2xl text-lg text-muted-foreground">No surprises. See exactly what you pay per transaction.</p>
                    </div>

                    <Card className="mb-16">
                        <CardContent className="p-8">
                            <div className="mb-8 flex items-center gap-4">
                                <div className="rounded-2xl bg-primary/10 p-4">
                                    <CreditCard className="h-10 w-10 text-primary" />
                                </div>
                                <div>
                                    <h3 className="text-2xl font-bold">Card Payments</h3>
                                    <p className="text-muted-foreground">Powered by Paystack</p>
                                </div>
                            </div>

                            <div className="space-y-4">
                                {RATES.map((rate) => (
                                    <div key={rate.name} className="border-t border-border pt-4 first:border-t-0 first:pt-0">
                                        <div className="flex items-start justify-between">
                                            <div>
                                                <h4 className="mb-1 font-semibold">{rate.name}</h4>
                                                <p className="text-sm text-muted-foreground">{rate.description}</p>
                                            </div>
                                            <span className="text-lg font-bold text-primary">{rate.fee}</span>
                                        </div>
                                    </div>
                                ))}
                            </div>
                        </CardContent>
                    </Card>

                    <PaymentCalculator />

                    <Card>
                        <CardContent className="grid gap-8 p-8 md:grid-cols-2 md:gap-12">
                            <div>
                                <div className="mb-3 flex items-center gap-2 text-primary">
                                    <ShieldCheck className="h-5 w-5" />
                                    <h3 className="text-xl font-bold text-foreground">Secure Payments</h3>
                                </div>
                                <p className="leading-relaxed text-muted-foreground">
                                    All transactions run through verified payment partners with industry-standard encryption.
                                </p>
                            </div>
                            <div className="md:border-l md:border-border md:pl-12">
                                <div className="mb-3 flex items-center gap-2 text-primary">
                                    <Clock3 className="h-5 w-5" />
                                    <h3 className="text-xl font-bold text-foreground">Cackle never holds your funds</h3>
                                </div>
                                <p className="leading-relaxed text-muted-foreground">
                                    Payouts settle directly through the payment provider — Cackle is never in the custody chain.
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
