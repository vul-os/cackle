import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Plus, Search, Calendar, Ticket, Edit, AlertCircle } from 'lucide-react';
import { format } from 'date-fns';
import { useAuth } from '@/context/use-auth';
import { events as eventsApi } from '@/lib/api';
import { toast } from '@/components/ui/use-toast';

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
    const [creating, setCreating] = useState(false);

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

    const handleCreateEvent = async () => {
        setCreating(true);
        try {
            const data = await eventsApi.create({ title: 'New Event' });
            const event = data?.event ?? data;
            navigate(`/admin/events/${event.id}`);
        } catch (err) {
            toast({ title: 'Could not create event', description: err.message, variant: 'destructive' });
        } finally {
            setCreating(false);
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
                <Button onClick={handleCreateEvent} disabled={creating}>
                    <Plus className="mr-2 h-4 w-4" />
                    {creating ? 'Creating...' : 'Create Event'}
                </Button>
            </div>

            <div className="relative mb-6">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input placeholder="Search events..." value={searchQuery} onChange={(e) => setSearchQuery(e.target.value)} className="pl-10" />
            </div>

            {state.loading && (
                <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
                    {[0, 1, 2].map((i) => (
                        <div key={i} className="h-48 animate-pulse rounded-xl bg-muted" />
                    ))}
                </div>
            )}

            {!state.loading && state.error && (
                <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-16 text-center">
                    <AlertCircle className="h-8 w-8 text-destructive" />
                    <p className="font-medium">{state.error}</p>
                    <Button variant="outline" onClick={fetchEvents}>
                        Try again
                    </Button>
                </div>
            )}

            {!state.loading && !state.error && filtered.length === 0 && (
                <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-16 text-center">
                    <Calendar className="h-8 w-8 text-muted-foreground" />
                    <p className="font-medium">{searchQuery ? 'No events match your search.' : 'No events yet'}</p>
                    {!searchQuery && <p className="text-sm text-muted-foreground">Create your first event to get started.</p>}
                </div>
            )}

            {!state.loading && !state.error && filtered.length > 0 && (
                <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
                    {filtered.map((event) => (
                        <Card key={event.id} className="flex cursor-pointer flex-col transition-shadow hover:shadow-lg" onClick={() => navigate(`/admin/events/${event.id}`)}>
                            <CardHeader>
                                <div className="flex items-center justify-between">
                                    <CardTitle className="truncate">{event.title}</CardTitle>
                                    <Badge variant={statusVariant[event.status] ?? 'secondary'}>{event.status ?? 'draft'}</Badge>
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
                                {event.summary && <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">{event.summary}</p>}
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
                                <Button size="sm" onClick={(e) => { e.stopPropagation(); navigate(`/admin/events/${event.id}`); }}>
                                    <Edit className="mr-2 h-4 w-4" />
                                    Edit
                                </Button>
                            </div>
                        </Card>
                    ))}
                </div>
            )}
        </div>
    );
};

export default EventsPage;
