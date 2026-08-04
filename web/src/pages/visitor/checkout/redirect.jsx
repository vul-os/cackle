import React, { useEffect } from 'react';
import PaymentStatusPage from '@/pages/visitor/payment/status';

export default function PaymentRedirectPage({ redirectUrl }) {
    useEffect(() => {
        const timer = setTimeout(() => {
            window.location.href = redirectUrl;
        }, 1200);
        return () => clearTimeout(timer);
    }, [redirectUrl]);

    return <PaymentStatusPage theme="redirecting" />;
}
