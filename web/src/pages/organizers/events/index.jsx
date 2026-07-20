import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Plus, Search, Calendar, Ticket, Edit, MoreVertical, Image as ImageIcon, Copy, Trash2 } from 'lucide-react';
import { format } from 'date-fns';
import { SkeletonCardGrid } from '@/components/ui/skeleton';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { useAuth } from '@/context/use-auth';
import { events as eventsApi, ticketTypes as ticketTypesApi } from '@/lib/api';
import { toast } from '@/components/ui/use-toast';
import DeleteEventDialog from './event/delete-dialog';
import ContinueDraftBanner from './continue-draft-banner';
import { categoryLabel } from './categories';
import { slugify } from './slug';
import { setPendingDraft } from './pending-draft';

const statusVariant = {
    draft: 'secondary',
    published: 'default',
    cancelled: 'destructive',
};

const EventsPage = () => {
    const navigate = useNavigate();
    const { activeOrg } = useAuth();
    const [state, setState] = useState({ events: [], loading: true, error: null });
    const [searchQuery, setSearchQuery] = useState('');
    const [duplicatingId, setDuplicatingId] = useState(null);
    const [deleteTarget, setDeleteTarget] = useState(null); // the event being confirmed for delete
    const [isDeleting, setIsDeleting] = useState(false);

    const fetchEvents = useCallback(() => {
        setState((s) => ({ ...s, loading: true, error: null }));
        eventsApi
            .list()
            .then((data) => {
                const list = Array.isArray(data) ? data : (data?.events ?? []);
                setState({ events: list, loading: false, error: null });
            })
            .catch((err) => setState({ events: [], loading: false, error: err.message || 'Could not load events.' }));
    }, []);

    useEffect(() => {
        fetchEvents();
    }, [fetchEvents]);

    const handleDuplicate = async (event) => {
        setDuplicatingId(event.id);
        try {
            const ttData = await ticketTypesApi.list(event.id);
            const sourceTicketTypes = Array.isArray(ttData) ? ttData : (ttData?.ticket_types ?? []);

            // Create requires starts_at/ends_at and a unique slug (see
            // internal/events.Service.Create) — a duplicate starts on the same
            // date/venue as the source event, which the organiser can then
            // change from the normal editor. Images aren't copied: stored image
            // files belong to the source event.
            const created = await eventsApi.create({
                org_id: activeOrg.id,
                slug: slugify(event.title),
                title: `${event.title || 'Untitled event'} (Copy)`,
                summary: event.summary || undefined,
                description: event.description || undefined,
                venue_name: event.venue_name || undefined,
                address: event.address || undefined,
                lat: event.lat ?? undefined,
                lng: event.lng ?? undefined,
                starts_at: event.starts_at,
                ends_at: event.ends_at,
                timezone: event.timezone || undefined,
                category: event.category || undefined,
                currency: event.currency || undefined,
            });
            const newEvent = created?.event ?? created;

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

            // The duplicate is a draft and — like any draft — invisible in the
            // Events list until published (see pending-draft.js); remember it
            // so there's a way back if the organiser navigates away first.
            setPendingDraft(activeOrg.id, newEvent.id);
            toast({ title: 'Duplicated', description: 'A new draft was created with the same details and ticket types.' });
            navigate(`/admin/events/${newEvent.id}`);
        } catch (err) {
            toast({ title: 'Could not duplicate', description: err.message, variant: 'destructive' });
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
            if (err.status === 404 || err.status === 405) {
                toast({
                    title: 'Not available yet',
                    description: 'Deleting events isn’t wired up on this server build. Set status to cancelled instead.',
                });
            } else {
                toast({ title: 'Could not delete', description: err.message, variant: 'destructive' });
            }
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
                    <Calendar className="h-8 w-8 text-primary" />
                    <div>
                        <h1 className="font-display text-3xl font-bold">Events</h1>
                        {activeOrg && <p className="text-sm text-muted-foreground">{activeOrg.name}</p>}
                    </div>
                </div>
                <Button onClick={() => navigate('/admin/events/new')}>
                    <Plus className="mr-2 h-4 w-4" />
                    Create Event
                </Button>
            </div>

            <ContinueDraftBanner />

            <div className="relative mb-6">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                    placeholder="Search events..."
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    className="pl-10"
                    aria-label="Search your events"
                />
            </div>

            {state.loading && <SkeletonCardGrid count={6} />}

            {!state.loading && state.error && <ErrorState description={state.error} onRetry={fetchEvents} />}

            {!state.loading && !state.error && filtered.length === 0 && (
                <EmptyState
                    icon={Calendar}
                    title={searchQuery ? 'No events match your search.' : 'No events yet'}
                    description={searchQuery ? 'Try a different search term.' : 'Create your first event to start selling tickets.'}
                    action={
                        !searchQuery && (
                            <Button size="sm" onClick={() => navigate('/admin/events/new')}>
                                <Plus className="mr-2 h-4 w-4" />
                                Create Event
                            </Button>
                        )
                    }
                />
            )}

            {!state.loading && !state.error && filtered.length > 0 && (
                <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
                    {filtered.map((event) => (
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
                                        <DropdownMenu>
                                            <DropdownMenuTrigger asChild>
                                                <Button
                                                    variant="ghost"
                                                    size="icon"
                                                    className="h-7 w-7"
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
                            </CardContent>
                            <div className="mt-auto flex justify-end gap-2 border-t border-border p-4">
                                <Button
                                    variant="outline"
                                    size="sm"
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
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        navigate(`/admin/events/${event.id}`);
                                    }}
                                >
                                    <Edit className="mr-2 h-4 w-4" />
                                    Edit
                                </Button>
                            </div>
                        </Card>
                    ))}
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
