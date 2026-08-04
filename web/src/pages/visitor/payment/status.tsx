import React from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { Loader2, CheckCircle2, XCircle, ExternalLink, Ticket, type LucideIcon } from 'lucide-react';
import Header from '@/pages/visitor/header';
import Footer from '@/pages/visitor/landing/footer';
import CheckoutSteps from '@/pages/visitor/checkout/steps';

interface PaymentThemeConfig {
    icon: LucideIcon;
    iconClass: string;
    ringClass: string;
    title: string;
    description: string;
    step: 'cart' | 'details' | 'pay' | 'ticket';
    busy?: boolean;
    showExternalLink?: boolean;
}

// One presentational surface for every terminal state of a payment. Each
// state says what happened, what it means, and — the part that was missing —
// what the buyer can DO next, including when it failed.
export const THEMES: Record<'processing' | 'success' | 'failed' | 'redirecting', PaymentThemeConfig> = {
    processing: {
        icon: Loader2,
        iconClass: 'text-primary-emphasis motion-safe:animate-spin',
        ringClass: 'bg-accent',
        title: 'Checking your payment',
        description: 'This usually takes a couple of seconds. Don’t close this page.',
        step: 'pay',
        busy: true,
    },
    success: {
        icon: CheckCircle2,
        iconClass: 'text-success',
        ringClass: 'bg-success/10',
        title: 'Payment received',
        description: 'You’re in. Your tickets are ready — each one has a QR code to show at the door.',
        step: 'ticket',
    },
    failed: {
        icon: XCircle,
        iconClass: 'text-destructive',
        ringClass: 'bg-destructive/10',
        title: 'We couldn’t confirm this payment',
        description:
            'Nothing has been issued. If money did leave your account, it will not have reached the organiser — check your orders before paying again.',
        step: 'pay',
    },
    redirecting: {
        icon: Loader2,
        iconClass: 'text-primary-emphasis motion-safe:animate-spin',
        ringClass: 'bg-accent',
        title: 'Taking you to pay',
        description: 'You’re being handed to the organiser’s payment page.',
        step: 'pay',
        busy: true,
        showExternalLink: true,
    },
};

export type PaymentTheme = keyof typeof THEMES;

interface PaymentStatusPageProps {
    theme?: PaymentTheme;
    /** the way forward from this state */
    actions?: React.ReactNode;
}

export default function PaymentStatusPage({ theme = 'processing', actions = null }: PaymentStatusPageProps) {
    const config = THEMES[theme] ?? THEMES.processing;
    const Icon = config.icon;
    const stepsProps = { current: config.step, className: 'mb-10' };

    return (
        <div className="flex min-h-screen flex-col bg-background">
            <Header />
            <main id="main" className="flex-1 pt-16">
                <div className="mx-auto max-w-lg px-4 py-10 sm:py-16">
                    <CheckoutSteps {...stepsProps} />

                    <Card
                        className="ticket-stub overflow-hidden"
                        style={{ '--notch': 'var(--background)', '--notch-y': '72%' } as React.CSSProperties}
                    >
                        <CardContent
                            className="px-6 pb-8 pt-10 text-center sm:px-8"
                            role="status"
                            aria-live="polite"
                            aria-busy={config.busy ? 'true' : 'false'}
                        >
                            <div className={`mx-auto flex h-20 w-20 items-center justify-center rounded-full ${config.ringClass}`}>
                                <Icon className={`h-10 w-10 ${config.iconClass}`} aria-hidden="true" />
                            </div>

                            <h1 className="mt-6 font-display text-display-sm font-extrabold tracking-tight">{config.title}</h1>
                            <p className="mx-auto mt-3 max-w-sm text-balance text-sm leading-relaxed text-muted-foreground">
                                {config.description}
                            </p>

                            {config.showExternalLink && (
                                <p className="mt-5 inline-flex items-center gap-2 text-xs text-muted-foreground">
                                    <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
                                    You&apos;ll be redirected automatically
                                </p>
                            )}

                            {actions && (
                                <>
                                    <Separator
                                        variant="perforated"
                                        className="my-7"
                                        style={{ '--notch': 'var(--background)' } as React.CSSProperties}
                                    />
                                    <div className="flex flex-col gap-3">{actions}</div>
                                </>
                            )}

                            {theme === 'success' && (
                                <p className="mt-6 flex items-center justify-center gap-2 text-xs text-muted-foreground">
                                    <Ticket className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                                    Your ticket works at the door with no signal.
                                </p>
                            )}
                        </CardContent>
                    </Card>
                </div>
            </main>
            <Footer />
        </div>
    );
}
