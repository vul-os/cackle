import React from 'react';
import { useNavigate, useLocation, Link } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Ghost, ArrowRight, Ticket } from 'lucide-react';
import { BrandLockup } from '@/components/brand/wordmark';

// This route is mounted twice (see routes.jsx): once under the bare public
// layout for any unmatched visitor URL, and once inside the organiser
// console's own chrome (top bar + side nav already visible) for any
// unmatched /admin/* URL. A single "Go home" button used to be the whole
// page in both cases — a dead end for a signed-in organiser whose typo
// lands them back on the marketing site, logged in but nowhere useful, and
// a page with zero brand presence for a visitor who followed a stale link
// and has no way to tell they are even still looking at Cackle.
//
// So this page now reads where it was mounted and offers the one thing
// that is actually useful from there:
//   - inside /admin/*, the console dashboard, not the public homepage
//   - everywhere else, the homepage plus one second route (what's on),
//     because "the page you wanted is gone" is more forgivable with two
//     doors out than one.
// It does not add a header/footer — the console context already has its
// own (a second one would double the nav), so the identity mark below is
// shown only on the bare public mount.
const NotFoundPage = () => {
    const navigate = useNavigate();
    const location = useLocation();
    const inConsole = location.pathname.startsWith('/admin');

    return (
        <div className="flex min-h-screen flex-col items-center justify-center gap-6 bg-background p-4 text-center text-foreground">
            {!inConsole && (
                <Link to="/" className="mb-2 flex h-11 items-center" aria-label="Cackle — home">
                    <BrandLockup size="lg" />
                </Link>
            )}

            <Ghost className="h-16 w-16 text-muted-foreground" aria-hidden="true" />

            <div className="space-y-2">
                <h1 className="font-display text-6xl font-black">404</h1>
                <p className="text-xl font-medium">Page not found</p>
                <p className="max-w-md text-muted-foreground">
                    {inConsole
                        ? 'That console page doesn’t exist. It may have moved, or the link was mistyped.'
                        : "The page you're looking for might have been removed, renamed, or never existed."}
                </p>
            </div>

            <div className="flex flex-wrap items-center justify-center gap-3">
                <Button className="min-w-[44px] px-6" onClick={() => navigate(inConsole ? '/admin' : '/')}>
                    {inConsole ? 'Go to dashboard' : 'Go home'}
                </Button>
                {!inConsole && (
                    <Button variant="outline" className="min-w-[44px] px-6" asChild>
                        <Link to="/events">
                            <Ticket className="mr-2 h-4 w-4" aria-hidden="true" />
                            Browse what&apos;s on
                            <ArrowRight className="ml-2 h-4 w-4" aria-hidden="true" />
                        </Link>
                    </Button>
                )}
            </div>
        </div>
    );
};

export default NotFoundPage;
