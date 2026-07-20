import React, { useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { ErrorState } from '@/components/ui/error-state';
import { Loader2, MailCheck } from 'lucide-react';
import { orgMembers as orgMembersApi } from '@/lib/api';
import { useAuth } from '@/context/use-auth';
import { toast } from '@/components/ui/use-toast';

/**
 * Lands here from an invite email's link (`/accept-invite?token=...`).
 * `ProtectedRoute` (see routes.jsx) already handles the "not signed in yet"
 * case by bouncing to /login with a returnTo back to this URL — by the
 * time this component renders, the caller is authenticated and all that's
 * left is the explicit "yes, join" confirmation.
 */
const AcceptInvitePage = () => {
    const [searchParams] = useSearchParams();
    const token = searchParams.get('token');
    const navigate = useNavigate();
    const { refresh } = useAuth();
    const [status, setStatus] = useState('idle'); // idle | accepting | done
    const [error, setError] = useState(null);

    const handleAccept = async () => {
        setStatus('accepting');
        setError(null);
        try {
            await orgMembersApi.acceptInvite(token);
            await refresh();
            setStatus('done');
            toast({ title: 'Welcome aboard', description: "You've joined the team." });
            setTimeout(() => navigate('/admin'), 1200);
        } catch (err) {
            setStatus('idle');
            setError(err.message || 'That invite link is invalid or has expired.');
        }
    };

    if (!token) {
        return (
            <div className="mx-auto max-w-md py-16">
                <ErrorState title="Missing invite link" description="This link is missing its invite token. Ask whoever invited you to resend it." />
            </div>
        );
    }

    return (
        <div className="mx-auto max-w-md py-16">
            <Card>
                <CardHeader className="text-center">
                    <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-primary">
                        <MailCheck className="h-6 w-6" />
                    </div>
                    <CardTitle>Join the team</CardTitle>
                    <CardDescription>You&apos;ve been invited to help organise events on Cackle.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                    {error && <p className="text-center text-sm font-medium text-destructive">{error}</p>}
                    {status === 'done' ? (
                        <p className="text-center text-sm text-muted-foreground">You&apos;re in — taking you to the dashboard…</p>
                    ) : (
                        <Button className="w-full" onClick={handleAccept} disabled={status === 'accepting'}>
                            {status === 'accepting' && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                            Accept invite
                        </Button>
                    )}
                </CardContent>
            </Card>
        </div>
    );
};

export default AcceptInvitePage;
