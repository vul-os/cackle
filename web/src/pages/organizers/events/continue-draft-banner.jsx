import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { PenLine, X, ArrowRight } from 'lucide-react';
import { useAuth } from '@/context/use-auth';
import { events as eventsApi } from '@/lib/api';
import { getPendingDraft, clearPendingDraft } from './pending-draft';

/**
 * "Continue setting up your event" banner — see pending-draft.js for why
 * this exists. Renders nothing if there's no pending draft, the draft was
 * already published (or deleted), or the lookup fails; never blocks the
 * page it's dropped into.
 */
const ContinueDraftBanner = () => {
    const { activeOrg } = useAuth();
    const navigate = useNavigate();
    const [draft, setDraft] = useState(null);

    useEffect(() => {
        const pendingId = getPendingDraft(activeOrg?.id);
        if (!pendingId) return;
        let cancelled = false;
        eventsApi
            .get(pendingId)
            .then((data) => {
                if (cancelled) return;
                const ev = data?.event ?? data;
                if (ev?.status === 'draft') {
                    setDraft(ev);
                } else {
                    // Already published, cancelled, or otherwise moved on —
                    // stop offering to resume it.
                    clearPendingDraft(activeOrg?.id);
                }
            })
            .catch(() => {
                if (cancelled) return;
                clearPendingDraft(activeOrg?.id);
            });
        return () => {
            cancelled = true;
        };
    }, [activeOrg?.id]);

    if (!draft) return null;

    const dismiss = () => {
        clearPendingDraft(activeOrg?.id);
        setDraft(null);
    };

    return (
        <Card className="mb-6 border-primary/30 bg-primary/5">
            <CardContent className="flex flex-col items-start justify-between gap-3 p-4 sm:flex-row sm:items-center">
                <div className="flex items-center gap-3">
                    <div className="rounded-full bg-primary/10 p-2 text-primary">
                        <PenLine className="h-4 w-4" />
                    </div>
                    <div>
                        <p className="text-sm font-medium">Continue setting up “{draft.title || 'your event'}”</p>
                        <p className="text-sm text-muted-foreground">Pick up where you left off in the event wizard.</p>
                    </div>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                    <Button size="sm" onClick={() => navigate(`/admin/events/${draft.id}/wizard`)}>
                        Resume
                        <ArrowRight className="ml-2 h-4 w-4" />
                    </Button>
                    <Button size="icon" variant="ghost" onClick={dismiss} aria-label="Dismiss">
                        <X className="h-4 w-4" />
                    </Button>
                </div>
            </CardContent>
        </Card>
    );
};

export default ContinueDraftBanner;
