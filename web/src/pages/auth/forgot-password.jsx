import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { motion } from 'framer-motion';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { KeyRound } from 'lucide-react';
import { copyTextToClipboard } from '@/pages/organizers/team/invite-link';
import { resetCommandFor } from './operator-reset';

import rockfestBg from '/images/rockfest.jpg';

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
        <div
            className="relative flex min-h-screen items-center justify-center p-4"
            style={{ backgroundImage: `url(${rockfestBg})`, backgroundSize: 'cover', backgroundPosition: 'center' }}
        >
            <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />
            <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.4 }} className="relative z-10 w-full max-w-md">
                <Card className="border-white/10 bg-card/95 shadow-2xl backdrop-blur">
                    <CardHeader className="text-center">
                        <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
                            <KeyRound className="h-6 w-6" />
                        </div>
                        <CardTitle className="font-display text-2xl font-bold">Forgot password</CardTitle>
                    </CardHeader>
                    <CardContent className="space-y-4">
                        <p className="text-sm text-muted-foreground">
                            Cackle does not send email, so there is no reset link on the way. Whoever runs this Cackle server can
                            make you one in a few seconds — ask them to run this on the machine it&apos;s installed on, and to send
                            you the link it prints.
                        </p>

                        <div className="space-y-2">
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

                        <div className="space-y-2">
                            <Label htmlFor="reset-command">Command to send them</Label>
                            <div className="flex flex-col gap-2 sm:flex-row">
                                <input
                                    id="reset-command"
                                    readOnly
                                    value={command}
                                    onFocus={(e) => e.target.select()}
                                    className="min-h-11 w-full flex-1 rounded-md border border-input bg-background px-3 py-2 font-mono text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                                />
                                <Button type="button" variant="outline" onClick={handleCopy} className="min-h-11 shrink-0 sm:w-28">
                                    {copied ? 'Copied' : 'Copy'}
                                </Button>
                            </div>
                            <p className="text-xs text-muted-foreground">
                                The link it prints works once and expires after an hour. If you run this server yourself, that
                                command is all you need.
                            </p>
                        </div>

                        <Button variant="outline" className="min-h-11 w-full" onClick={() => navigate('/login')}>
                            Back to sign in
                        </Button>
                    </CardContent>
                </Card>
            </motion.div>
        </div>
    );
};

export default ForgotPassword;
