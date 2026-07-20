import React from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Loader2, CheckCircle2, XCircle, ExternalLink } from 'lucide-react';
import Header from '@/pages/visitor/header';

const THEMES = {
    processing: {
        icon: Loader2,
        iconClass: 'text-primary animate-spin',
        title: 'Verifying Payment',
        description: 'Please wait while we confirm your payment...',
        buttonText: 'Please wait...',
        buttonDisabled: true,
    },
    success: {
        icon: CheckCircle2,
        iconClass: 'text-success',
        title: 'Payment Successful!',
        description: 'Thank you for your purchase. Your order has been confirmed.',
        buttonText: 'View My Orders',
        buttonDisabled: false,
    },
    failed: {
        icon: XCircle,
        iconClass: 'text-destructive',
        title: 'Payment Failed',
        description: 'We were unable to process your payment. Please try again.',
        buttonText: 'Back to Orders',
        buttonDisabled: false,
    },
    redirecting: {
        icon: Loader2,
        iconClass: 'text-primary animate-spin',
        title: 'Redirecting to Payment Gateway',
        description: 'Please wait while we redirect you to our secure payment page...',
        buttonText: 'Redirecting...',
        buttonDisabled: true,
        showExternalLink: true,
    },
};

export default function PaymentStatusPage({ theme = 'processing', onButtonClick = () => {} }) {
    const config = THEMES[theme] ?? THEMES.processing;
    const Icon = config.icon;

    return (
        <>
            <Header />
            <div className="min-h-screen bg-background">
                <div className="container mx-auto max-w-lg px-4 py-32">
                    <Card>
                        <CardContent className="pb-8 pt-12">
                            <div className="flex flex-col items-center space-y-8 text-center">
                                <Icon className={`h-20 w-20 ${config.iconClass}`} />
                                <div className="space-y-2">
                                    <h1 className="text-2xl font-semibold tracking-tight">{config.title}</h1>
                                    <p className="text-base text-muted-foreground">{config.description}</p>
                                </div>
                                {config.showExternalLink && (
                                    <div className="flex items-center gap-2 text-sm text-muted-foreground">
                                        <ExternalLink className="h-4 w-4" />
                                        <span>You will be redirected automatically</span>
                                    </div>
                                )}
                                <Button className="w-full max-w-xs" disabled={config.buttonDisabled} onClick={onButtonClick}>
                                    {config.buttonText}
                                </Button>
                            </div>
                        </CardContent>
                    </Card>
                </div>
            </div>
        </>
    );
}
