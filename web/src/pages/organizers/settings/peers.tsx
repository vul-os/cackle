import { useCallback, useEffect, useState } from 'react';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { Form, FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { SkeletonList } from '@/components/ui/skeleton';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { toast } from '@/components/ui/use-toast';
import { Link2, KeyRound, RefreshCw, Trash2, ExternalLink } from 'lucide-react';
import { useAuth } from '@/context/use-auth';
import { request } from '@/lib/api';

// The federation ("other organisers") endpoints below (/sync/status,
// /sync/peers*) are not part of lib/api-types.ts — that file mirrors the
// documented DTOs in docs/API.md, and this feature predates/sits outside it.
// These shapes are taken directly from the Go response structs they wire to
// (internal/httpapi/sync_handlers.go's syncPeerView/syncStatusResponse,
// internal/httpapi/peer_feed.go's feedPullResult/peerEventView), kept local
// to this page rather than invented.

interface SyncPeer {
    id: string;
    name?: string;
    url?: string;
    public_key: string;
    enabled: boolean;
    dialable: boolean;
    pull_cursor: number;
    push_cursor: number;
    last_sync_at?: string;
    last_status?: string;
    created_at: string;
    feed_publish: boolean;
    feed_subscribe: boolean;
    feed_pulled_at?: string;
    feed_status?: string;
}

interface SyncStatusResponse {
    node: string;
    algebra: string;
    engine?: string;
    peers: SyncPeer[];
    caveat: string;
    standalone: boolean;
}

interface PeerEventView {
    external: boolean;
    publisher: string;
    publisher_key: string;
    url: string;
    title: string;
    summary?: string;
    venue_name?: string;
    address?: string;
    starts_at: string;
    ends_at: string;
    timezone?: string;
    category?: string;
    fetched_at: string;
    notice: string;
}

interface PeerEventsResponse {
    events: PeerEventView[];
    caveat: string;
}

interface FeedPullResult {
    peer_id: string;
    peer: string;
    fetched: number;
    stored: number;
    refused?: string[];
    pages: number;
    complete: boolean;
    error?: string;
    caveat: string;
}

// Settings ▸ Other organisers.
//
// This screen is where a host decides, one organiser at a time, whether to let
// them list this host's events and whether to list theirs. It is deliberately
// dull: two checkboxes, a "fetch now" button, and a plain list of what came
// back. There is nothing to browse and nothing to search, because there is
// nothing to browse — Cackle has no directory of organisers and this page must
// never imply otherwise.
//
// The two questions are separate on purpose. Letting someone see your programme
// and choosing to show theirs are different decisions, and a screen that folded
// them into one switch would be quietly deciding one of them for the operator.

const enrolSchema = z.object({
    name: z.string().trim().max(80, 'Keep the label short.').optional(),
    url: z
        .string()
        .trim()
        .min(1, 'Paste the address they gave you.')
        // Deliberately no example URL in this message. A literal address in
        // shipped app source is what check-app.mjs is watching for, and it is
        // right to: the app must not carry addresses of its own.
        .refine((v) => /^https?:\/\/[^/\s]+\/?$/.test(v), 'Their site address on its own, starting with http, with nothing after the host.'),
    public_key: z
        .string()
        .trim()
        .min(1, 'Paste the key they gave you.')
        .refine((v) => /^[0-9a-fA-F]{64}$/.test(v), 'A key is 64 characters, digits and the letters a–f.'),
});

function shortKey(key: string | null | undefined): string {
    if (!key) return '';
    return `${key.slice(0, 8)}…${key.slice(-4)}`;
}

function whenText(iso: string | null | undefined): string {
    if (!iso) return 'never';
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return 'never';
    return d.toLocaleString();
}

interface PeerRowProps {
    peer: SyncPeer;
    onChanged: () => Promise<void>;
}

/** One enrolled organiser: the two switches, a fetch button, and what it brought back. */
function PeerRow({ peer, onChanged }: PeerRowProps) {
    const [busy, setBusy] = useState(false);
    const [listings, setListings] = useState<PeerEventView[] | null>(null);

    const loadListings = useCallback(async () => {
        if (!peer.feed_subscribe) {
            setListings(null);
            return;
        }
        try {
            const data = await request<PeerEventsResponse>(`/sync/peers/${peer.id}/feed`);
            setListings(data?.events ?? []);
        } catch {
            setListings([]);
        }
    }, [peer.id, peer.feed_subscribe]);

    useEffect(() => {
        void loadListings();
    }, [loadListings]);

    const setSwitches = async (publish: boolean, subscribe: boolean) => {
        setBusy(true);
        try {
            await request(`/sync/peers/${peer.id}/feed`, {
                method: 'PUT',
                body: { publish, subscribe },
            });
            await onChanged();
        } catch (err) {
            toast({ variant: 'destructive', title: 'That did not save', description: err instanceof Error ? err.message : undefined });
        } finally {
            setBusy(false);
        }
    };

    const fetchNow = async () => {
        setBusy(true);
        try {
            const out = await request<FeedPullResult>(`/sync/peers/${peer.id}/feed`, { method: 'POST' });
            // A big programme arrives over several requests. The count is what an
            // operator cares about, but "we did not get all of it" is the thing
            // they must not have to infer from a number looking round — so it is
            // said, in words, and the title changes with it.
            const across = out.pages > 1 ? ` over ${out.pages} requests` : '';
            const cut = out.complete
                ? ''
                : ' This organiser publishes more events than one fetch can hold, so this is not their whole programme.';
            toast({
                title: out.complete ? 'Fetched' : 'Fetched — but not all of it',
                description:
                    `Showing ${out.stored} of ${out.fetched} event${out.fetched === 1 ? '' : 's'}` +
                    ` from this organiser${across}.${cut}`,
            });
            await onChanged();
            await loadListings();
        } catch (err) {
            // A refusal is the important case, so it is shown in full rather
            // than flattened into "something went wrong".
            toast({ variant: 'destructive', title: 'Refused', description: err instanceof Error ? err.message : undefined });
        } finally {
            setBusy(false);
        }
    };

    const remove = async () => {
        setBusy(true);
        try {
            await request(`/sync/peers/${peer.id}`, { method: 'DELETE' });
            toast({ title: 'Removed', description: 'Their events are no longer shown, and they can no longer see yours.' });
            await onChanged();
        } catch (err) {
            toast({ variant: 'destructive', title: 'Could not remove', description: err instanceof Error ? err.message : undefined });
        } finally {
            setBusy(false);
        }
    };

    return (
        <div className="space-y-4 p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                    <p className="truncate text-base font-medium">{peer.name || 'Unnamed organiser'}</p>
                    <p className="mt-1 break-all font-mono text-xs text-muted-foreground">{shortKey(peer.public_key)}</p>
                    {peer.url ? <p className="mt-1 break-all text-xs text-muted-foreground">{peer.url}</p> : null}
                </div>
                <Badge variant={peer.enabled ? 'secondary' : 'outline'}>{peer.enabled ? 'Enrolled' : 'Paused'}</Badge>
            </div>

            <div className="grid gap-3 sm:grid-cols-2">
                <label className="flex min-h-[44px] cursor-pointer items-center gap-3 rounded-md border border-border p-3">
                    <Checkbox
                        checked={peer.feed_publish}
                        disabled={busy}
                        onCheckedChange={(v) => setSwitches(Boolean(v), peer.feed_subscribe)}
                        aria-label="Let this organiser list my events"
                    />
                    <span className="text-sm">
                        <span className="block font-medium">They can list my events</span>
                        <span className="block text-xs text-muted-foreground">Only events you have published.</span>
                    </span>
                </label>

                <label className="flex min-h-[44px] cursor-pointer items-center gap-3 rounded-md border border-border p-3">
                    <Checkbox
                        checked={peer.feed_subscribe}
                        disabled={busy || !peer.dialable}
                        onCheckedChange={(v) => setSwitches(peer.feed_publish, Boolean(v))}
                        aria-label="Show this organiser's events on my site"
                    />
                    <span className="text-sm">
                        <span className="block font-medium">Show their events here</span>
                        <span className="block text-xs text-muted-foreground">
                            {peer.dialable ? 'Fetched only when you ask.' : 'Needs their address, which you did not enter.'}
                        </span>
                    </span>
                </label>
            </div>

            <div className="flex flex-wrap items-center gap-2">
                <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={busy || !peer.feed_subscribe}
                    onClick={fetchNow}
                >
                    <RefreshCw className="mr-2 h-4 w-4" />
                    Fetch their events now
                </Button>
                <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={remove}>
                    <Trash2 className="mr-2 h-4 w-4" />
                    Remove
                </Button>
            </div>

            <p className="text-xs text-muted-foreground">
                Last fetched {whenText(peer.feed_pulled_at)}
                {peer.feed_status ? ` — ${peer.feed_status}` : ''}
            </p>

            {peer.feed_subscribe && listings ? (
                listings.length === 0 ? (
                    <p className="text-xs text-muted-foreground">Nothing fetched from this organiser yet.</p>
                ) : (
                    <ul className="space-y-2">
                        {listings.map((e) => (
                            <li key={e.url} className="rounded-md border border-border p-3">
                                <p className="text-sm font-medium">{e.title}</p>
                                <p className="mt-1 text-xs text-muted-foreground">
                                    {e.publisher} · {new Date(e.starts_at).toLocaleString()}
                                </p>
                                <p className="mt-1 text-xs text-muted-foreground">{e.notice}</p>
                                <a
                                    href={e.url}
                                    target="_blank"
                                    rel="noreferrer noopener"
                                    className="mt-2 inline-flex min-h-[44px] items-center gap-1 text-xs underline"
                                >
                                    Open on their site
                                    <ExternalLink className="h-3 w-3" />
                                </a>
                            </li>
                        ))}
                    </ul>
                )
            ) : null}
        </div>
    );
}

interface PeersState {
    loading: boolean;
    error: Error | null;
    node: string;
    peers: SyncPeer[];
}

const PeersPage = () => {
    const { activeOrg } = useAuth();
    const orgId = activeOrg?.id;

    const [state, setState] = useState<PeersState>({ loading: true, error: null, node: '', peers: [] });

    const load = useCallback(async () => {
        if (!orgId) {
            setState({ loading: false, error: null, node: '', peers: [] });
            return;
        }
        setState((s) => ({ ...s, loading: true, error: null }));
        try {
            const data = await request<SyncStatusResponse>(`/sync/status?org=${encodeURIComponent(orgId)}`);
            setState({ loading: false, error: null, node: data?.node ?? '', peers: data?.peers ?? [] });
        } catch (err) {
            setState({ loading: false, error: err instanceof Error ? err : new Error(String(err)), node: '', peers: [] });
        }
    }, [orgId]);

    useEffect(() => {
        void load();
    }, [load]);

    const form = useForm<{ name?: string; url: string; public_key: string }>({
        resolver: zodResolver(enrolSchema),
        defaultValues: { name: '', url: '', public_key: '' },
    });

    const enrol = async (values: { name?: string; url: string; public_key: string }) => {
        try {
            await request('/sync/peers', {
                method: 'POST',
                body: {
                    org_id: orgId,
                    name: values.name ?? '',
                    url: values.url.replace(/\/+$/, ''),
                    public_key: values.public_key.toLowerCase(),
                },
            });
            form.reset();
            toast({
                title: 'Added',
                description: 'Nothing is shared yet. Choose what to share with them below.',
            });
            await load();
        } catch (err) {
            toast({ variant: 'destructive', title: 'Could not add them', description: err instanceof Error ? err.message : undefined });
        }
    };

    if (!orgId) {
        return (
            <div className="mx-auto max-w-3xl space-y-6">
                <h1 className="font-display text-3xl font-bold">Other organisers</h1>
                <EmptyState
                    icon={Link2}
                    title="No organisation yet"
                    description="Create an organisation first, then you can share events with other organisers you know."
                />
            </div>
        );
    }

    return (
        <div className="mx-auto max-w-3xl space-y-6">
            <div>
                <h1 className="font-display text-3xl font-bold">Other organisers</h1>
                <p className="mt-2 text-sm text-muted-foreground">
                    You can let organisers you know list your events, and show theirs beside yours. You decide both, one organiser
                    at a time, and you can change your mind at any time.
                </p>
            </div>

            <Card>
                <CardHeader>
                    <CardTitle className="text-lg">How this works</CardTitle>
                </CardHeader>
                <CardContent className="space-y-2 text-sm text-muted-foreground">
                    <p>
                        Cackle does not look for other organisers and has no list of them. You add someone by swapping two things
                        with them directly — your address and your key, below — the same way you would swap phone numbers.
                    </p>
                    <p>
                        When you show someone else&apos;s event, it is a signpost. Their event stays on their own site, their
                        tickets are sold there, and the money goes to them. You are never selling their tickets.
                    </p>
                </CardContent>
            </Card>

            <Card>
                <CardHeader>
                    <CardTitle className="text-lg">Your key</CardTitle>
                    <CardDescription>Send this, and your address, to the organiser you want to share with.</CardDescription>
                </CardHeader>
                <CardContent>
                    <div className="flex items-start gap-3 rounded-md border border-border p-3">
                        <KeyRound className="mt-1 h-4 w-4 shrink-0 text-muted-foreground" />
                        <code className="min-w-0 break-all font-mono text-xs">{state.node || '—'}</code>
                    </div>
                </CardContent>
            </Card>

            <Card>
                <CardHeader>
                    <CardTitle className="text-lg">Add an organiser</CardTitle>
                    <CardDescription>Paste the address and key they sent you.</CardDescription>
                </CardHeader>
                <CardContent>
                    <Form {...form}>
                        <form onSubmit={form.handleSubmit(enrol)} className="space-y-4">
                            <FormField
                                control={form.control}
                                name="name"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>What to call them</FormLabel>
                                        <FormControl>
                                            <Input placeholder="The hall down the road" {...field} />
                                        </FormControl>
                                        <FormDescription>Just a label for you. It is not checked against anything.</FormDescription>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={form.control}
                                name="url"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Their address</FormLabel>
                                        <FormControl>
                                            <Input placeholder="Paste the address they sent you" inputMode="url" {...field} />
                                        </FormControl>
                                        <FormDescription>
                                            The address of their site on its own — nothing after the host name.
                                        </FormDescription>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={form.control}
                                name="public_key"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Their key</FormLabel>
                                        <FormControl>
                                            <Input placeholder="64 characters" className="font-mono" {...field} />
                                        </FormControl>
                                        <FormDescription>
                                            Check it with them out of band. If the key ever stops matching, Cackle refuses to talk
                                            to that address rather than trusting it again.
                                        </FormDescription>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <Button type="submit" className="w-full sm:w-auto" disabled={form.formState.isSubmitting}>
                                Add them
                            </Button>
                        </form>
                    </Form>
                </CardContent>
            </Card>

            <Card>
                <CardHeader>
                    <CardTitle className="text-lg">Organisers you have added</CardTitle>
                    <CardDescription>Nothing is shared until you tick a box.</CardDescription>
                </CardHeader>
                <CardContent className="p-0">
                    {state.loading ? (
                        <div className="p-4">
                            <SkeletonList rows={2} />
                        </div>
                    ) : state.error ? (
                        <div className="p-4">
                            <ErrorState
                                title="Could not load your organisers"
                                description={state.error?.message}
                                onRetry={load}
                            />
                        </div>
                    ) : state.peers.length === 0 ? (
                        <div className="p-4">
                            <EmptyState
                                icon={Link2}
                                title="No one added yet"
                                description="Swap addresses and keys with an organiser you know, then add them above."
                            />
                        </div>
                    ) : (
                        <div className="divide-y divide-border">
                            {state.peers.map((peer) => (
                                <PeerRow key={peer.id} peer={peer} onChanged={load} />
                            ))}
                        </div>
                    )}
                </CardContent>
            </Card>
        </div>
    );
};

export default PeersPage;
