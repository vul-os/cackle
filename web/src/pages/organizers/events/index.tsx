import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge, type BadgeProps } from '@/components/ui/badge';
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Plus, Search, Calendar, Ticket, Edit, Eye, MoreVertical, Image as ImageIcon, Copy, Trash2, ShieldCheck } from 'lucide-react';
import { format } from 'date-fns';
import { SkeletonCardGrid } from '@/components/ui/skeleton';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { Money } from '@/components/ui/money';
import { useAuth } from '@/context/use-auth';
import { events as eventsApi, ticketTypes as ticketTypesApi } from '@/lib/api';
import { toast } from '@/components/ui/use-toast';
import DeleteEventDialog from './event/delete-dialog';
import { categoryLabel } from './categories';
import { slugify } from './slug';
import type { CackleEvent, EventStats } from '@/lib/api-types';

const statusVariant: Record<string, BadgeProps['variant']> = {
    draft: 'secondary',
    published: 'default',
    cancelled: 'destructive',
};

interface EventsPageState {
    events: CackleEvent[];
    loading: boolean;
    error: string | null;
}

const EventsPage = () => {
    const navigate = useNavigate();
    const { activeOrg } = useAuth();
    // Owner/admin can create, edit, duplicate and delete events; a scanner
    // can list and read every one of them (server RBAC is scanner+ for the
    // listing itself) but is 403'd on every write. Ticket-type management
    // in particular requires admin+ even to READ, so a scanner following a
    // "Tickets" link into that page would hit a wall on load, not just on
    // save — every admin-only action below is gated out for them instead
    // of being shown and left to fail.
    const canManage = activeOrg?.role === 'owner' || activeOrg?.role === 'admin';

    const [state, setState] = useState<EventsPageState>({ events: [], loading: true, error: null });
    const [statsById, setStatsById] = useState<Record<string, EventStats>>({});
    const [searchQuery, setSearchQuery] = useState('');
    const [duplicatingId, setDuplicatingId] = useState<string | null>(null);
    const [deleteTarget, setDeleteTarget] = useState<CackleEvent | null>(null); // the event being confirmed for delete
    const [isDeleting, setIsDeleting] = useState(false);

    const fetchEvents = useCallback(() => {
        if (!activeOrg?.id) {
            setState({ events: [], loading: false, error: null });
            return;
        }
        setState((s) => ({ ...s, loading: true, error: null }));
        eventsApi
            .listForOrg(activeOrg.id)
            .then(async (data) => {
                const list = data.events ?? [];
                setState({ events: list, loading: false, error: null });

                // Best-effort per-event stats so the list can show sold/
                // admitted counts. One event's stats failing to load just
                // leaves that card without a count rather than blanking the
                // whole list.
                const results = await Promise.allSettled(list.map((ev) => eventsApi.stats(ev.id)));
                const next: Record<string, EventStats> = {};
                results.forEach((r, i) => {
                    const ev = list[i];
                    if (ev && r.status === 'fulfilled') next[ev.id] = r.value.stats;
                });
                setStatsById(next);
            })
            .catch((err) => setState({ events: [], loading: false, error: err?.message || 'Could not load events.' }));
    }, [activeOrg?.id]);

    useEffect(() => {
        fetchEvents();
    }, [fetchEvents]);

    const handleDuplicate = async (event: CackleEvent) => {
        setDuplicatingId(event.id);
        try {
            const ttData = await ticketTypesApi.list(event.id);
            const sourceTicketTypes = ttData.ticket_types ?? [];

            // Create requires starts_at/ends_at and a unique slug (see
            // internal/events.Service.Create) — a duplicate starts on the same
            // date/venue as the source event, which the organiser can then
            // change from the normal editor. Images aren't copied: stored image
            // files belong to the source event. CreateEventInput's string
            // fields are required (not optional, unlike UpdateEventInput) —
            // sending the value (or '' for cover_image, deliberately not
            // carried over) directly rather than coalescing falsy values to
            // undefined; both produce the same zero value once Go decodes it.
            const created = await eventsApi.create({
                org_id: activeOrg!.id,
                slug: slugify(event.title),
                title: `${event.title || 'Untitled event'} (Copy)`,
                summary: event.summary,
                description: event.description,
                venue_name: event.venue_name,
                address: event.address,
                lat: event.lat ?? undefined,
                lng: event.lng ?? undefined,
                starts_at: event.starts_at,
                ends_at: event.ends_at,
                timezone: event.timezone,
                cover_image: '',
                category: event.category,
                currency: event.currency,
            });
            const newEvent = created.event;

            await Promise.all(
                sourceTicketTypes.map((tt) =>
                    ticketTypesApi.create(newEvent.id, {
                        name: tt.name,
                        description: tt.description || undefined,
                        price_minor: tt.price_minor,
                        quantity_total: tt.quantity_total,
                        max_per_order: tt.max_per_order,
                        sales_start: tt.sales_start,
                        sales_end: tt.sales_end,
                        sort_order: tt.sort_order,
                    }),
                ),
            );

            toast({ title: 'Duplicated', description: 'A new draft was created with the same details and ticket types.' });
            navigate(`/admin/events/${newEvent.id}`);
        } catch (err) {
            const message = err instanceof Error ? err.message : undefined;
            toast({ title: 'Could not duplicate', description: message, variant: 'destructive' });
        } finally {
            setDuplicatingId(null);
        }
    };

    const handleDelete = async () => {
        if (!deleteTarget) return;
        setIsDeleting(true);
        try {
            await eventsApi.remove(deleteTarget.id);
            toast({ title: 'Deleted', description: 'The event has been removed.' });
            setState((s) => ({ ...s, events: s.events.filter((e) => e.id !== deleteTarget.id) }));
            setDeleteTarget(null);
        } catch (err) {
            // 409 conflict: the event has issued tickets — the server steers
            // toward cancelling instead (see docs/API.md). Its message
            // already says so; just surface it rather than a generic one.
            const message = err instanceof Error ? err.message : undefined;
            toast({ title: 'Could not delete', description: message, variant: 'destructive' });
        } finally {
            setIsDeleting(false);
        }
    };

    const filtered = state.events.filter(
        (e) =>
            !searchQuery ||
            e.title?.toLowerCase().includes(searchQuery.toLowerCase()) ||
            e.venue_name?.toLowerCase().includes(searchQuery.toLowerCase()),
    );

    return (
        <div className="mx-auto max-w-6xl">
            <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-center gap-3">
                    <Calendar className="h-8 w-8 text-primary-emphasis" />
                    <div>
                        <h1 className="font-display text-3xl font-bold">Events</h1>
                        {activeOrg && <p className="text-sm text-muted-foreground">{activeOrg.name}</p>}
                    </div>
                </div>
                {canManage && (
                    <Button className="min-h-11" onClick={() => navigate('/admin/events/new')}>
                        <Plus className="mr-2 h-4 w-4" />
                        Create Event
                    </Button>
                )}
            </div>

            <div className="relative mb-6">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                    placeholder="Search events..."
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    className="min-h-11 pl-10"
                    aria-label="Search your events"
                />
            </div>

            {state.loading && <SkeletonCardGrid count={6} />}

            {!state.loading && state.error && <ErrorState description={state.error} onRetry={fetchEvents} />}

            {!state.loading && !state.error && filtered.length === 0 && (
                <EmptyState
                    icon={Calendar}
                    title={searchQuery ? 'No events match your search.' : 'No events yet'}
                    description={
                        searchQuery
                            ? 'Try a different search term.'
                            : canManage
                              ? 'Create your first event to start selling tickets.'
                              : 'Ask an owner or admin to create one to get started.'
                    }
                    action={
                        !searchQuery && canManage ? (
                            <Button size="sm" className="min-h-11" onClick={() => navigate('/admin/events/new')}>
                                <Plus className="mr-2 h-4 w-4" />
                                Create Event
                            </Button>
                        ) : undefined
                    }
                />
            )}

            {!state.loading && !state.error && filtered.length > 0 && (
                <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
                    {filtered.map((event) => {
                        const s = statsById[event.id];
                        return (
                            <Card
                                key={event.id}
                                className="flex cursor-pointer flex-col transition-shadow hover:shadow-lg"
                                onClick={() => navigate(`/admin/events/${event.id}`)}
                            >
                                <CardHeader>
                                    <div className="flex items-start justify-between gap-2">
                                        <CardTitle className="truncate">{event.title}</CardTitle>
                                        <div className="flex shrink-0 items-center gap-1">
                                            <Badge variant={statusVariant[event.status] ?? 'secondary'}>{event.status ?? 'draft'}</Badge>
                                            {canManage && (
                                                <DropdownMenu>
                                                    <DropdownMenuTrigger asChild>
                                                        {/* Was h-7 w-7 (28×28) — under the 44×44 floor at 390px, and this
                                                            is the ONLY route to Duplicate/Delete on a card. The negative
                                                            margin keeps the glyph's optical position roughly where it
                                                            was rather than pushing the card title further left. */}
                                                        <Button
                                                            variant="ghost"
                                                            size="icon"
                                                            className="-my-2.5 -mr-2 h-11 w-11"
                                                            onClick={(e) => e.stopPropagation()}
                                                            aria-label={`More actions for ${event.title}`}
                                                        >
                                                            <MoreVertical className="h-4 w-4" />
                                                        </Button>
                                                    </DropdownMenuTrigger>
                                                    <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
                                                        <DropdownMenuItem onClick={() => navigate(`/admin/events/${event.id}`)}>
                                                            <Edit className="mr-2 h-4 w-4" />
                                                            Edit
                                                        </DropdownMenuItem>
                                                        <DropdownMenuItem onClick={() => navigate(`/admin/events/${event.id}/images`)}>
                                                            <ImageIcon className="mr-2 h-4 w-4" />
                                                            Images
                                                        </DropdownMenuItem>
                                                        <DropdownMenuItem disabled={duplicatingId === event.id} onClick={() => handleDuplicate(event)}>
                                                            <Copy className="mr-2 h-4 w-4" />
                                                            {duplicatingId === event.id ? 'Duplicating…' : 'Duplicate'}
                                                        </DropdownMenuItem>
                                                        <DropdownMenuSeparator />
                                                        <DropdownMenuItem
                                                            onClick={() => setDeleteTarget(event)}
                                                            className="text-destructive focus:text-destructive"
                                                        >
                                                            <Trash2 className="mr-2 h-4 w-4" />
                                                            Delete
                                                        </DropdownMenuItem>
                                                    </DropdownMenuContent>
                                                </DropdownMenu>
                                            )}
                                        </div>
                                    </div>
                                    {event.starts_at && (
                                        <CardDescription className="flex items-center gap-1.5">
                                            <Calendar className="h-3.5 w-3.5" />
                                            {format(new Date(event.starts_at), 'PPP')}
                                        </CardDescription>
                                    )}
                                </CardHeader>
                                <CardContent className="flex-1">
                                    {event.venue_name && <p className="text-sm text-muted-foreground">{event.venue_name}</p>}
                                    {event.category && (
                                        <Badge variant="outline" className="mt-2">
                                            {categoryLabel(event.category)}
                                        </Badge>
                                    )}
                                    {event.summary && <p className="mt-2 line-clamp-2 text-sm text-muted-foreground">{event.summary}</p>}
                                    {/* Status is on the badge above; sold/admitted is the
                                        other thing an organiser scans this list for. */}
                                    <p className="mt-3 flex items-center gap-3 text-sm text-muted-foreground">
                                        {s ? (
                                            <>
                                                <span className="flex items-center gap-1.5">
                                                    <Ticket className="h-3.5 w-3.5" />
                                                    {s.sold ?? 0} sold
                                                </span>
                                                <span className="flex items-center gap-1.5">
                                                    <ShieldCheck className="h-3.5 w-3.5" />
                                                    {s.admitted ?? 0} admitted
                                                </span>
                                            </>
                                        ) : (
                                            '—'
                                        )}
                                    </p>
                                    {s && s.revenue_minor > 0 && (
                                        <p className="mt-1 text-sm text-muted-foreground">
                                            <Money minor={s.revenue_minor} currency={event.currency} /> revenue
                                        </p>
                                    )}
                                </CardContent>
                                {canManage ? (
                                    <div className="mt-auto flex justify-end gap-2 border-t border-border p-4">
                                        <Button
                                            variant="outline"
                                            size="sm"
                                            className="min-h-11"
                                            onClick={(e) => {
                                                e.stopPropagation();
                                                navigate(`/admin/events/${event.id}/tickets`);
                                            }}
                                        >
                                            <Ticket className="mr-2 h-4 w-4" />
                                            Tickets
                                        </Button>
                                        <Button
                                            size="sm"
                                            className="min-h-11"
                                            onClick={(e) => {
                                                e.stopPropagation();
                                                navigate(`/admin/events/${event.id}`);
                                            }}
                                        >
                                            <Edit className="mr-2 h-4 w-4" />
                                            Edit
                                        </Button>
                                    </div>
                                ) : (
                                    <div className="mt-auto flex justify-end border-t border-border p-4">
                                        <Button
                                            variant="outline"
                                            size="sm"
                                            className="min-h-11"
                                            onClick={(e) => {
                                                e.stopPropagation();
                                                navigate(`/admin/events/${event.id}`);
                                            }}
                                        >
                                            <Eye className="mr-2 h-4 w-4" />
                                            View
                                        </Button>
                                    </div>
                                )}
                            </Card>
                        );
                    })}
                </div>
            )}

            <DeleteEventDialog
                open={!!deleteTarget}
                onOpenChange={(open) => !open && setDeleteTarget(null)}
                eventTitle={deleteTarget?.title}
                onConfirm={handleDelete}
                isDeleting={isDeleting}
            />
        </div>
    );
};

export default EventsPage;
