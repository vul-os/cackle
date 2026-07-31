import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { SkeletonList } from '@/components/ui/skeleton';
import { QrCode, Download, RefreshCw, Wifi, WifiOff, CheckCircle2, Radio } from 'lucide-react';
import { events as eventsApi, scan as scanApi } from '@/lib/api';
import { saveBundle, getBundle, listCachedBundles } from '@/lib/scan-store';
import { publicKeyToBytes } from '@/lib/capability';
import { useOnline } from '@/lib/use-online';
import { toast } from '@/components/ui/use-toast';
import ScanView from './scan-view';

const GATE_ID_KEY = 'cackle_gate_id';

// The backend's scan-bundle wraps signing keys as
// `issuer_keys: { event_id, keys: { <kid>: "<base64url pubkey>" } }`
// (tickets.KeyRing's own JSON encoding) — a map keyed by kid, not an array.
function buildKeyRing(issuerKeys) {
    const ring = {};
    const keys = issuerKeys?.keys ?? {};
    for (const [kid, encoded] of Object.entries(keys)) {
        try {
            ring[kid] = publicKeyToBytes(encoded);
        } catch {
            // skip a key we can't decode rather than fail the whole bundle
        }
    }
    return ring;
}

// The bundle's `event` is a reduced scan.EventMeta (event_id/title/venue_name
// /starts_at/ends_at only), not the full Event — and it keys on `event_id`,
// not `id`. Normalise against whatever richer event object we already have
// (from the organizer's event list) so the rest of the UI can just use `.id`.
function normaliseSessionEvent(bundleEvent, fallbackEvent) {
    return {
        ...fallbackEvent,
        ...bundleEvent,
        id: bundleEvent?.event_id ?? fallbackEvent?.id,
    };
}

const ScannerPage = () => {
    const online = useOnline();
    const navigate = useNavigate();
    const [state, setState] = useState({ events: [], loading: true, error: null });
    const [cachedIds, setCachedIds] = useState(new Set());
    const [downloadingId, setDownloadingId] = useState(null);
    const [session, setSession] = useState(null); // { event, keyRing }
    const [gateId, setGateId] = useState(() => localStorage.getItem(GATE_ID_KEY) || 'Gate 1');

    useEffect(() => {
        localStorage.setItem(GATE_ID_KEY, gateId);
    }, [gateId]);

    const refreshCached = async () => {
        const bundles = await listCachedBundles();
        setCachedIds(new Set(bundles.map((b) => b.event_id)));
        return bundles;
    };

    // Factored out (rather than inlined in the mount effect) so the error
    // state's "Try again" button runs exactly the same offline-first
    // fallback as the initial load — a gate that fails once should get the
    // same behaviour on retry, not a second code path that might disagree.
    // `cancelledRef` lets a retry started before an earlier call resolves
    // (or an unmount) avoid clobbering newer state.
    const loadEvents = useCallback((cancelledRef) => {
        setState((s) => ({ ...s, loading: true, error: null }));
        refreshCached();
        eventsApi
            .list()
            .then((data) => {
                if (cancelledRef?.current) return;
                const list = Array.isArray(data) ? data : (data?.events ?? []);
                setState({ events: list, loading: false, error: null });
            })
            .catch(async (err) => {
                if (cancelledRef?.current) return;
                // Offline (or the network call otherwise failed) — fall back
                // entirely to whatever bundles are already cached locally, so
                // a gate device that's never coming back online can still work.
                const bundles = await refreshCached();
                if (cancelledRef?.current) return;
                setState({
                    events: bundles
                        .map((b) => (b.event ? { ...b.event, id: b.event.event_id } : null))
                        .filter(Boolean),
                    loading: false,
                    error: bundles.length === 0 ? err.message || 'Could not load events.' : null,
                });
            });
    }, []);

    useEffect(() => {
        const cancelledRef = { current: false };
        loadEvents(cancelledRef);
        return () => {
            cancelledRef.current = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const handleDownload = async (event) => {
        setDownloadingId(event.id);
        try {
            const bundle = await scanApi.bundle(event.id);
            await saveBundle(event.id, bundle);
            await refreshCached();
            toast({ title: 'Scan bundle ready', description: `${event.title} is cached for offline scanning.` });
        } catch (err) {
            toast({ title: 'Download failed', description: err.message, variant: 'destructive' });
        } finally {
            setDownloadingId(null);
        }
    };

    const handleEnterScanMode = async (event) => {
        let bundle = await getBundle(event.id);
        if (!bundle && online) {
            try {
                bundle = await scanApi.bundle(event.id);
                await saveBundle(event.id, bundle);
            } catch (err) {
                toast({ title: 'Could not download scan bundle', description: err.message, variant: 'destructive' });
                return;
            }
        }
        if (!bundle) {
            toast({
                title: "You're offline",
                description: 'Download the scan bundle for this event while online first.',
                variant: 'destructive',
            });
            return;
        }
        const keyRing = buildKeyRing(bundle.issuer_keys);
        // ticket_index (see docs/OFFLINE-GATES.md) is the set of ticket ids
        // currently valid for this event as of when the bundle was
        // downloaded. ticket_index_present says whether that set is
        // AUTHORITATIVE: a server-built bundle always sets it true, so an
        // empty index means "admit nothing" (all revoked / none issued), NOT
        // "no data". Only a legacy bundle lacking the flag falls back to
        // signature-only checking. This mirrors Go's DecideWithBundle exactly.
        //
        // admitted_index is the tickets the server already had an admission
        // for when this bundle was built — the ONLY way this device learns
        // about an admission that happened at a DIFFERENT gate. It needs no
        // "present" flag: empty and absent both mean "this bundle knows of
        // nobody already inside", and both correctly leave the decision to the
        // local dedupe log. See internal/scan/bundle.go.
        setSession({
            event: normaliseSessionEvent(bundle.event, event),
            keyRing,
            ticketIndex: Array.isArray(bundle.ticket_index) ? bundle.ticket_index : [],
            ticketIndexPresent: bundle.ticket_index_present === true,
            admittedIndex: Array.isArray(bundle.admitted_index) ? bundle.admitted_index : [],
        });
    };

    const cachedAsFallback = useMemo(
        () => state.events.length === 0 && cachedIds.size > 0,
        [state.events, cachedIds],
    );

    if (session) {
        return (
            <ScanView
                event={session.event}
                keyRing={session.keyRing}
                ticketIndex={session.ticketIndex}
                ticketIndexPresent={session.ticketIndexPresent}
                admittedIndex={session.admittedIndex}
                gateId={gateId}
                onExit={() => setSession(null)}
            />
        );
    }

    return (
        <div className="mx-auto max-w-3xl">
            <div className="mb-8 flex items-center gap-3">
                <QrCode className="h-8 w-8 text-primary-emphasis" />
                <div>
                    <h1 className="font-display text-3xl font-bold">Gate Scanner</h1>
                    <p className="text-muted-foreground">Download a scan bundle once, then admit guests with the network unplugged.</p>
                </div>
            </div>

            <Card className="mb-6">
                <CardContent className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between">
                    <div className="flex items-center gap-2">
                        <div
                            className={`flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-semibold ${
                                // Badge wash + on-wash ink, same idiom as
                                // ConnectionState's `positive` class: the
                                // *-foreground tokens are tuned for text ON a
                                // solid fill, not on a low-alpha wash, so
                                // pairing warning/15 with warning-foreground
                                // (near-white or near-black by theme) would
                                // wash out to near-invisible here. The base
                                // success/warning tokens are themselves
                                // legible ink at this alpha.
                                online ? 'bg-success/15 text-success' : 'bg-warning/15 text-warning'
                            }`}
                        >
                            {online ? <Wifi className="h-3.5 w-3.5" /> : <WifiOff className="h-3.5 w-3.5" />}
                            {online ? 'Online — bundles can be downloaded' : "Offline — you'll need a cached bundle"}
                        </div>
                    </div>
                    <div className="flex items-center gap-2">
                        <Label htmlFor="gate-id" className="whitespace-nowrap text-sm text-muted-foreground">
                            <Radio className="mr-1 inline h-3.5 w-3.5" />
                            Gate name
                        </Label>
                        {/* h-11 (44px): a gate operator is naming their own
                            door, on a phone, possibly wearing gloves. */}
                        <Input
                            id="gate-id"
                            value={gateId}
                            onChange={(e) => setGateId(e.target.value)}
                            className="h-11 w-32 sm:h-11"
                        />
                    </div>
                </CardContent>
            </Card>

            {state.loading && <SkeletonList rows={2} />}

            {!state.loading && state.error && !cachedAsFallback && (
                <ErrorState description={state.error} onRetry={() => loadEvents()} />
            )}

            {!state.loading && state.events.length === 0 && !state.error && (
                <EmptyState
                    icon={QrCode}
                    title="No events to scan yet"
                    description="Publish an event first, then come back here to prep the gate."
                    action={
                        <Button size="lg" onClick={() => navigate('/admin/events/new')}>
                            Create an event
                        </Button>
                    }
                />
            )}

            {!state.loading && state.events.length > 0 && (
                <div className="space-y-3">
                    {state.events.map((event) => {
                        const isCached = cachedIds.has(event.id);
                        return (
                            <Card key={event.id}>
                                <CardContent className="flex flex-col gap-3 p-5 sm:flex-row sm:items-center sm:justify-between">
                                    <div className="min-w-0">
                                        <div className="flex items-center gap-2">
                                            <p className="truncate font-medium">{event.title}</p>
                                            {isCached && (
                                                <Badge variant="secondary" className="gap-1">
                                                    <CheckCircle2 className="h-3 w-3" />
                                                    Cached
                                                </Badge>
                                            )}
                                        </div>
                                        {event.venue_name && <p className="text-sm text-muted-foreground">{event.venue_name}</p>}
                                    </div>
                                    <div className="flex gap-2">
                                        <Button
                                            variant="outline"
                                            onClick={() => handleDownload(event)}
                                            disabled={!online || downloadingId === event.id}
                                        >
                                            {downloadingId === event.id ? (
                                                <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                                            ) : (
                                                <Download className="mr-2 h-4 w-4" />
                                            )}
                                            {isCached ? 'Refresh' : 'Download'}
                                        </Button>
                                        <Button
                                            onClick={() => handleEnterScanMode(event)}
                                            disabled={!isCached && !online}
                                        >
                                            <QrCode className="mr-2 h-4 w-4" />
                                            Scan
                                        </Button>
                                    </div>
                                </CardContent>
                            </Card>
                        );
                    })}
                </div>
            )}
        </div>
    );
};

export default ScannerPage;
