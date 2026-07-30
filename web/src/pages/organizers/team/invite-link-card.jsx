import React, { useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Check, Copy, KeyRound, X } from 'lucide-react';
import { copyTextToClipboard } from './invite-link';

/**
 * The invite link, shown once, for the owner to send themselves.
 *
 * Three things this component must never stop doing, because each of them
 * is the fix for a specific way the old flow lied:
 *
 *  1. DISPLAY the link as selectable text. Copy-to-clipboard is a
 *     convenience on top; it needs a secure context and a self-hosted
 *     Cackle on a venue LAN is usually plain http. If the button fails the
 *     owner can still select the link with their mouse, so the flow has no
 *     dead end.
 *  2. Say the expiry in plain language. An invite is valid for 7 days
 *     (orgs.DefaultInviteTTL) and a link that quietly stopped working is
 *     the kind of thing you discover at 8pm with a queue outside.
 *  3. Say that it is shown once, and why. The server stores only a sha256
 *     hash of the token (internal/orgs.CreateInvite), so "show it again"
 *     is not a feature that was skipped — it is a thing that cannot be
 *     built without downgrading a credential to plaintext at rest. The
 *     recovery path is revoke-and-reinvite, and it is stated here.
 */
const InviteLinkCard = ({ url, email, roleLabel, expiresAt, onDismiss }) => {
    const [copied, setCopied] = useState(false);
    const [copyFailed, setCopyFailed] = useState(false);
    const inputRef = useRef(null);

    const handleCopy = async () => {
        const ok = await copyTextToClipboard(url);
        setCopied(ok);
        setCopyFailed(!ok);
        if (ok) {
            setTimeout(() => setCopied(false), 3000);
        } else {
            // Select it for them so the manual copy is one keystroke.
            inputRef.current?.focus();
            inputRef.current?.select();
        }
    };

    return (
        <Card className="border-primary/40 bg-accent/40">
            <CardHeader className="flex flex-row items-start justify-between gap-3 space-y-0">
                <CardTitle className="flex items-center gap-2 text-base">
                    <KeyRound className="h-4 w-4 shrink-0 text-primary-emphasis" />
                    Send this link to {email}
                </CardTitle>
                {onDismiss && (
                    <button
                        type="button"
                        onClick={onDismiss}
                        aria-label="Dismiss invite link"
                        className="-m-2.5 flex h-11 w-11 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
                    >
                        <X className="h-4 w-4" />
                    </button>
                )}
            </CardHeader>
            <CardContent className="space-y-3">
                <p className="text-sm text-muted-foreground">
                    Cackle does not send email. Copy this link and give it to them however you already talk — WhatsApp, SMS, or
                    read it out.
                </p>

                <div className="flex flex-col gap-2 sm:flex-row">
                    <input
                        ref={inputRef}
                        readOnly
                        value={url}
                        aria-label="Invite link"
                        onFocus={(e) => e.target.select()}
                        className="min-h-11 w-full flex-1 rounded-md border border-input bg-background px-3 py-2 font-mono text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    />
                    <Button type="button" onClick={handleCopy} className="min-h-11 shrink-0 sm:w-36">
                        {copied ? <Check className="mr-2 h-4 w-4" /> : <Copy className="mr-2 h-4 w-4" />}
                        {copied ? 'Copied' : 'Copy link'}
                    </Button>
                </div>

                {copyFailed && (
                    <p className="text-xs text-destructive">
                        Could not reach the clipboard — the link is selected above, press Ctrl/Cmd+C to copy it.
                    </p>
                )}

                <ul className="space-y-1.5 text-xs text-muted-foreground">
                    <li>
                        They join as <span className="font-medium text-foreground">{roleLabel}</span>, and must sign in with{' '}
                        <span className="font-medium text-foreground">{email}</span> — the link only works for that address.
                    </li>
                    {expiresAt && (
                        <li>
                            The link stops working on <span className="font-medium text-foreground">{expiresAt}</span>.
                        </li>
                    )}
                    <li>
                        This is the only time the link is shown. Cackle keeps just a scrambled copy of it, so it cannot be
                        looked up again — if you lose it, revoke the invite below and make a new one.
                    </li>
                </ul>
            </CardContent>
        </Card>
    );
};

export default InviteLinkCard;
