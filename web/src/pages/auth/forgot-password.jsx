import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Clock } from 'lucide-react';
import { copyTextToClipboard } from '@/pages/organizers/team/invite-link';
import { resetCommandFor } from './operator-reset';
import { AuthShell } from './auth-shell';

const RAIL_POINTS = [{ icon: Clock, text: 'The link they hand back to you works once, and expires after an hour.' }];

/**
 * Forgot password, told truthfully.
 *
 * This page used to take an email, POST it, and toast a confirmation that
 * a reset message had gone out and to go look in the mailbox. Nothing was
 * ever sent, because Cackle contains no mail code at all; the token the
 * server minted went nowhere and
 * /update-password was reachable by nobody. Somebody acting on that
 * message waits for a message that will never arrive and stays locked
 * out — the most costly kind of false statement this product can make.
 *
 * What is true is that a self-hosted single binary has an operator, and
 * `cackle reset-password -email you@example.com` prints a working link in
 * about two seconds. So the page's job is to name that path and hand over
 * the exact command, rather than to fake a mailbox.
 *
 * The form no longer calls the API. POST /api/auth/password-reset still
 * exists (see docs/API.md) but mints a token nothing can deliver, so
 * calling it would only fill the table with unusable rows and imply
 * something happened.
 */
const ForgotPassword = () => {
    const [email, setEmail] = useState('');
    const [copied, setCopied] = useState(false);
    const navigate = useNavigate();

    const command = resetCommandFor(email);

    const handleCopy = async () => {
        const ok = await copyTextToClipboard(command);
        setCopied(ok);
        if (ok) setTimeout(() => setCopied(false), 3000);
    };

    return (
        <AuthShell
            eyebrow="Forgot password"
            title="Ask the operator."
            description="Cackle does not send email, so no reset link is coming from anywhere. Whoever runs this box can make you one in seconds."
            points={RAIL_POINTS}
            formHeading="Forgot password"
            formSubheading="Type your email, then hand the command below to whoever operates this Cackle server."
        >
            <div className="space-y-5">
                <div className="space-y-1.5">
                    <Label htmlFor="email">Your email address</Label>
                    <Input
                        id="email"
                        type="email"
                        value={email}
                        placeholder="you@example.com"
                        onChange={(e) => setEmail(e.target.value)}
                        className="min-h-11"
                    />
                </div>

                <div className="space-y-1.5">
                    <Label htmlFor="reset-command">Command to send them</Label>
                    <div className="flex flex-col gap-2 sm:flex-row">
                        <input
                            id="reset-command"
                            readOnly
                            value={command}
                            onFocus={(e) => e.target.select()}
                            className="min-h-11 w-full flex-1 rounded-md border border-input bg-background px-3 py-2 font-mono text-xs text-foreground shadow-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
                        />
                        <Button type="button" variant="outline" onClick={handleCopy} className="min-h-11 shrink-0 sm:w-28">
                            {copied ? 'Copied' : 'Copy'}
                        </Button>
                    </div>
                    <p className="text-xs text-muted-foreground">
                        Run on the machine Cackle is installed on. The link it prints works once and expires after an hour — that
                        command is all a self-hosted operator needs.
                    </p>
                </div>

                <div className="border-t border-border pt-2">
                    <Button variant="outline" className="min-h-11 w-full" onClick={() => navigate('/login')}>
                        Back to sign in
                    </Button>
                </div>
            </div>
        </AuthShell>
    );
};

export default ForgotPassword;
