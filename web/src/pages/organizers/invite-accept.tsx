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
 * Lands here from the invite link an organiser sent by hand
 * (`/accept-invite?token=...`). Cackle sends no email: the owner copies
 * this link off their Team page and passes it on over WhatsApp, SMS or
 * whatever they already use — see `team/invite-link.js` for why.
 *
 * `ProtectedRoute` (see routes.tsx) already handles the "not signed in
 * yet" case by bouncing to /login, storing `pathname + search` so the
 * token survives a signup detour — by the time this component renders,
 * the caller is authenticated and all that's left is the explicit "yes,
 * join" confirmation.
 *
 * The server additionally requires that the signed-in account's own email
 * matches the address the invite names, so a forwarded link cannot be
 * redeemed by the wrong person. That is stated up front here rather than
 * left to surface as a 403 after they have already made an account.
 */
const AcceptInvitePage = () => {
    const [searchParams] = useSearchParams();
    const token = searchParams.get('token');
    const navigate = useNavigate();
    const { refresh, user } = useAuth();
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
                <ErrorState
                    title="Missing invite link"
                    description="This link is missing its invite code — it was probably cut short when it was sent. Ask whoever invited you to send you the whole link again."
                />
            </div>
        );
    }

    return (
        <div className="mx-auto max-w-md py-16">
            <Card>
                <CardHeader className="text-center">
                    <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-primary-emphasis">
                        <MailCheck className="h-6 w-6" />
                    </div>
                    <CardTitle>Join the team</CardTitle>
                    <CardDescription>
                        You&apos;ve been invited to help run events on Cackle. You must be signed in as the email address the
                        invite was sent to — {user?.email ? `you are signed in as ${user.email}.` : 'sign in with that address first.'}
                    </CardDescription>
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
