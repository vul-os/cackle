import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { toast } from '@/components/ui/use-toast';
import { payments as paymentsApi } from '@/lib/api';
import PaymentStatusPage from './status';

export default function PaymentConfirmationPage() {
    const navigate = useNavigate();
    const [searchParams] = useSearchParams();
    const [status, setStatus] = useState('processing'); // 'processing' | 'success' | 'failed'

    useEffect(() => {
        const reference = searchParams.get('reference');
        if (!reference) {
            setStatus('failed');
            toast({ title: 'Verification failed', description: 'No payment reference found', variant: 'destructive' });
            return;
        }

        let cancelled = false;
        paymentsApi
            .verify(reference)
            .then(() => {
                if (cancelled) return;
                setStatus('success');
                toast({ title: 'Payment successful', description: 'Your order has been confirmed.' });
            })
            .catch((err) => {
                if (cancelled) return;
                setStatus('failed');
                toast({ title: 'Payment failed', description: err.message || 'We could not verify this payment.', variant: 'destructive' });
            });

        return () => {
            cancelled = true;
        };
    }, [searchParams]);

    return <PaymentStatusPage theme={status} onButtonClick={() => navigate('/orders')} />;
}
