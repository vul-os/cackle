import React, { useEffect, useMemo, useState } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { QrCode, Download, RefreshCw, Wifi, WifiOff, CheckCircle2, AlertCircle, Radio } from 'lucide-react';
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

    useEffect(() => {
        let cancelled = false;

        refreshCached();

        eventsApi
            .list()
            .then((data) => {
                if (cancelled) return;
                const list = Array.isArray(data) ? data : (data?.events ?? []);
                setState({ events: list, loading: false, error: null });
            })
            .catch(async (err) => {
                if (cancelled) return;
                // Offline (or the network call otherwise failed) — fall back
                // entirely to whatever bundles are already cached locally, so
                // a gate device that's never coming back online can still work.
                const bundles = await refreshCached();
                setState({
                    events: bundles
                        .map((b) => (b.event ? { ...b.event, id: b.event.event_id } : null))
                        .filter(Boolean),
                    loading: false,
                    error: bundles.length === 0 ? err.message || 'Could not load events.' : null,
                });
            });

        return () => {
            cancelled = true;
        };
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
        setSession({
            event: normaliseSessionEvent(bundle.event, event),
            keyRing,
            ticketIndex: Array.isArray(bundle.ticket_index) ? bundle.ticket_index : [],
            ticketIndexPresent: bundle.ticket_index_present === true,
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
                gateId={gateId}
                onExit={() => setSession(null)}
            />
        );
    }

    return (
        <div className="mx-auto max-w-3xl">
            <div className="mb-8 flex items-center gap-3">
                <QrCode className="h-8 w-8 text-primary" />
                <div>
                    <h1 className="font-display text-3xl font-bold">Gate Scanner</h1>
                    <p className="text-muted-foreground">Download a scan bundle once, then admit guests with the network unplugged.</p>
                </div>
            </div>

            <Card className="mb-6">
                <CardContent className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between">
                    <div className="flex items-center gap-2">
                        <div
                            className={`flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold ${
                                online ? 'bg-success/15 text-success' : 'bg-warning/15 text-warning-foreground'
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
                        <Input id="gate-id" value={gateId} onChange={(e) => setGateId(e.target.value)} className="h-8 w-32" />
                    </div>
                </CardContent>
            </Card>

            {state.loading && (
                <div className="space-y-3">
                    {[0, 1].map((i) => (
                        <div key={i} className="h-20 animate-pulse rounded-xl bg-muted" />
                    ))}
                </div>
            )}

            {!state.loading && state.error && !cachedAsFallback && (
                <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-16 text-center">
                    <AlertCircle className="h-8 w-8 text-destructive" />
                    <p className="font-medium">{state.error}</p>
                </div>
            )}

            {!state.loading && state.events.length === 0 && !state.error && (
                <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-16 text-center">
                    <QrCode className="h-8 w-8 text-muted-foreground" />
                    <p className="font-medium">No events to scan yet</p>
                    <p className="text-sm text-muted-foreground">Publish an event first, then come back here to prep the gate.</p>
                </div>
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
                                            size="sm"
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
                                        <Button size="sm" onClick={() => handleEnterScanMode(event)} disabled={!isCached && !online}>
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
