import React, { useEffect, useState } from 'react';
import { LogIn } from 'lucide-react';
import { request } from '@/lib/api';
import { Button } from '@/components/ui/button';
import { signInProvidersFrom } from './sso';

// The sign-in provider row.
//
// It renders NOTHING at all unless this box's operator configured a provider.
// That is not a styling choice: `GET /api/auth/providers` answers with an
// empty list on a stock Cackle, the OAuth routes are not mounted, and a
// button pointing at a route that does not exist is a dead end dressed up as
// a feature.
//
// It also renders nothing while the answer is still in flight, and nothing if
// the request fails. Every non-answer means the same thing here — no
// buttons — so an offline box, an unconfigured box and a broken request all
// land in the identical, working state: the password form above, alone.
//
// The password form is never hidden, moved or de-emphasised for this. It is
// the only sign-in that works when the internet does not, which on this
// product is the normal case rather than the exception.
const SsoButtons = ({ disabled = false }) => {
    const [providers, setProviders] = useState([]);

    useEffect(() => {
        let live = true;
        // Called through the shared `request` wrapper rather than added to
        // the `auth` bundle in lib/api.js: this is the only caller, and one
        // import is a smaller surface than a new exported method.
        request('/auth/providers', { skipAuthRedirect: true })
            .then((data) => {
                if (live) setProviders(signInProvidersFrom(data));
            })
            .catch(() => {
                // No signal, no provider list, no buttons. Deliberately
                // silent: nothing is wrong from the user's point of view —
                // the form they need is already on screen.
                if (live) setProviders([]);
            });
        return () => {
            live = false;
        };
    }, []);

    if (providers.length === 0) return null;

    return (
        <div className="space-y-3">
            <div className="flex items-center gap-3" aria-hidden="true">
                <span className="h-px flex-1 bg-border" />
                <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">or</span>
                <span className="h-px flex-1 bg-border" />
            </div>
            {providers.map((p) => (
                <Button key={p.id} asChild variant="outline" size="lg" className="w-full text-sm">
                    {/* A path on this box, never a URL at the provider — the
                        server does the redirecting, so the app itself still
                        reaches out to nobody. */}
                    <a href={disabled ? undefined : p.startPath} aria-disabled={disabled || undefined}>
                        <LogIn className="h-4 w-4" aria-hidden="true" />
                        Continue with {p.label}
                    </a>
                </Button>
            ))}
        </div>
    );
};

export default SsoButtons;
