import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Calendar, Plus, QrCode, Ticket, Building2 } from 'lucide-react';
import { useAuth } from '@/context/use-auth';
import { events as eventsApi } from '@/lib/api';

const HomePage = () => {
    const navigate = useNavigate();
    const { activeOrg, orgs } = useAuth();
    const [state, setState] = useState({ events: [], loading: true });

    useEffect(() => {
        let cancelled = false;
        eventsApi
            .list()
            .then((data) => {
                if (cancelled) return;
                setState({ events: Array.isArray(data) ? data : (data?.events ?? []), loading: false });
            })
            .catch(() => setState({ events: [], loading: false }));
        return () => {
            cancelled = true;
        };
    }, []);

    if (!orgs || orgs.length === 0) {
        return (
            <div className="mx-auto flex max-w-lg flex-col items-center gap-4 py-24 text-center">
                <Building2 className="h-12 w-12 text-muted-foreground" />
                <h1 className="font-display text-2xl font-bold">No organization yet</h1>
                <p className="text-muted-foreground">
                    Your account isn&apos;t attached to an organizer profile yet. This usually happens automatically at signup — try
                    signing out and back in, or contact support if it persists.
                </p>
            </div>
        );
    }

    const upcoming = state.events.filter((e) => e.status === 'published').slice(0, 3);
    const drafts = state.events.filter((e) => e.status === 'draft').length;

    return (
        <div className="mx-auto max-w-6xl">
            <div className="mb-10">
                <div className="mb-2 flex items-center gap-3">
                    <Ticket className="h-8 w-8 text-primary" />
                    <h1 className="font-display text-3xl font-bold sm:text-4xl">{activeOrg?.name ?? 'Your events'}</h1>
                </div>
                <p className="text-muted-foreground">Manage events, sell tickets, and run the gate — all from here.</p>
            </div>

            <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
                <Card className="transition-shadow hover:shadow-lg">
                    <CardHeader>
                        <CardTitle className="flex items-center gap-2">
                            <Plus className="h-5 w-5" />
                            Create an event
                        </CardTitle>
                        <CardDescription>Get a new event up in seconds.</CardDescription>
                    </CardHeader>
                    <CardContent>
                        <p className="text-sm text-muted-foreground">Set your dates, ticket types and go live whenever you&apos;re ready.</p>
                    </CardContent>
                    <CardFooter>
                        <Button className="w-full" onClick={() => navigate('/admin/events')}>
                            Go to Events
                        </Button>
                    </CardFooter>
                </Card>

                <Card className="transition-shadow hover:shadow-lg">
                    <CardHeader>
                        <CardTitle className="flex items-center gap-2">
                            <QrCode className="h-5 w-5" />
                            Scan the gate
                        </CardTitle>
                        <CardDescription>Works fully offline.</CardDescription>
                    </CardHeader>
                    <CardContent>
                        <p className="text-sm text-muted-foreground">Download the scan bundle once while online, then admit guests with no signal.</p>
                    </CardContent>
                    <CardFooter>
                        <Button variant="outline" className="w-full" onClick={() => navigate('/admin/scanner')}>
                            Open Scanner
                        </Button>
                    </CardFooter>
                </Card>

                <Card>
                    <CardHeader>
                        <CardTitle>Quick stats</CardTitle>
                        <CardDescription>Your event pipeline</CardDescription>
                    </CardHeader>
                    <CardContent className="space-y-3">
                        <div className="flex items-center justify-between">
                            <span className="text-sm text-muted-foreground">Total events</span>
                            <span className="font-semibold">{state.loading ? '—' : state.events.length}</span>
                        </div>
                        <div className="flex items-center justify-between">
                            <span className="text-sm text-muted-foreground">Published</span>
                            <span className="font-semibold">{state.loading ? '—' : state.events.filter((e) => e.status === 'published').length}</span>
                        </div>
                        <div className="flex items-center justify-between">
                            <span className="text-sm text-muted-foreground">Drafts</span>
                            <span className="font-semibold">{state.loading ? '—' : drafts}</span>
                        </div>
                    </CardContent>
                </Card>
            </div>

            <div className="mt-10">
                <h2 className="mb-4 text-xl font-semibold">Upcoming events</h2>
                {state.loading ? (
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                        {[0, 1, 2].map((i) => (
                            <div key={i} className="h-32 animate-pulse rounded-xl bg-muted" />
                        ))}
                    </div>
                ) : upcoming.length === 0 ? (
                    <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-border py-12 text-center text-muted-foreground">
                        <Calendar className="h-8 w-8" />
                        <p>No published events yet.</p>
                    </div>
                ) : (
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                        {upcoming.map((event) => (
                            <Card key={event.id} className="cursor-pointer hover:shadow-md" onClick={() => navigate(`/admin/events/${event.id}`)}>
                                <CardContent className="p-5">
                                    <p className="truncate font-medium">{event.title}</p>
                                    {event.venue_name && <p className="mt-1 text-sm text-muted-foreground">{event.venue_name}</p>}
                                </CardContent>
                            </Card>
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
};

export default HomePage;
