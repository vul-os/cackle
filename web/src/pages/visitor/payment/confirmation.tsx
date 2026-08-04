import React, { useCallback, useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/use-toast';
import { payments as paymentsApi } from '@/lib/api';
import PaymentStatusPage from './status';
import { Ticket, Receipt, RotateCcw } from 'lucide-react';

/**
 * Where a payment provider drops the buyer back. It verifies the reference
 * and then has to leave them somewhere they can act — the previous version
 * offered a single "View My Orders" button in every state, including the
 * failed one, with no way to retry a verification that lost a race with the
 * provider's webhook.
 */
export default function PaymentConfirmationPage() {
    const [searchParams] = useSearchParams();
    const reference = searchParams.get('reference');
    const [status, setStatus] = useState(reference ? 'processing' : 'failed');
    const [attempt, setAttempt] = useState(0);

    const retry = useCallback(() => {
        if (!reference) return;
        setStatus('processing');
        setAttempt((n) => n + 1);
    }, [reference]);

    useEffect(() => {
        if (!reference) {
            toast({
                title: 'Nothing to verify',
                description: 'This link is missing its payment reference.',
                variant: 'destructive',
            });
            return;
        }

        let cancelled = false;
        paymentsApi
            .verify(reference)
            .then(() => {
                if (cancelled) return;
                setStatus('success');
                toast({ title: 'Payment received', description: 'Your tickets are ready.' });
            })
            .catch((err) => {
                if (cancelled) return;
                setStatus('failed');
                toast({
                    title: 'Could not confirm payment',
                    description: err.message || 'We could not verify this payment.',
                    variant: 'destructive',
                });
            });

        return () => {
            cancelled = true;
        };
    }, [reference, attempt]);

    const actions =
        status === 'success' ? (
            <>
                <Button size="lg" asChild>
                    <Link to="/tickets">
                        <Ticket className="mr-2 h-4 w-4" aria-hidden="true" />
                        See my tickets
                    </Link>
                </Button>
                <Button variant="outline" asChild>
                    <Link to="/orders">
                        <Receipt className="mr-2 h-4 w-4" aria-hidden="true" />
                        View the order
                    </Link>
                </Button>
            </>
        ) : status === 'failed' ? (
            <>
                {/* A verification can lose the race with the provider's own
                    webhook, so "try again" is a real remedy here, not a
                    placebo — it re-runs the same verify call. */}
                {reference && (
                    <Button size="lg" onClick={retry}>
                        <RotateCcw className="mr-2 h-4 w-4" aria-hidden="true" />
                        Try again
                    </Button>
                )}
                <Button variant="outline" asChild>
                    <Link to="/orders">
                        <Receipt className="mr-2 h-4 w-4" aria-hidden="true" />
                        Check my orders
                    </Link>
                </Button>
            </>
        ) : null;

    return <PaymentStatusPage theme={status} actions={actions} />;
}
