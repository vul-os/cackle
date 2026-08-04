import React, { useEffect } from 'react';
import PaymentStatusPage from '@/pages/visitor/payment/status';

export interface PaymentRedirectPageProps {
    redirectUrl: string;
}

export default function PaymentRedirectPage({ redirectUrl }: PaymentRedirectPageProps) {
    useEffect(() => {
        const timer = setTimeout(() => {
            window.location.href = redirectUrl;
        }, 1200);
        return () => clearTimeout(timer);
    }, [redirectUrl]);

    return <PaymentStatusPage theme="redirecting" />;
}
