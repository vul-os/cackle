import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { ArrowLeft, AlertCircle, Ticket, Coins, ShieldCheck, BarChart3 } from 'lucide-react';
import { events as eventsApi } from '@/lib/api';

function formatMoney(cents, currency = 'ZAR') {
    try {
        return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format((cents || 0) / 100);
    } catch {
        return `${((cents || 0) / 100).toFixed(2)} ${currency}`;
    }
}

const StatTile = ({ icon: Icon, label, value }) => (
    <Card>
        <CardContent className="flex items-center gap-4 p-6">
            <div className="rounded-xl bg-primary/10 p-3 text-primary">
                <Icon className="h-6 w-6" />
            </div>
            <div>
                <p className="text-sm text-muted-foreground">{label}</p>
                <p className="text-2xl font-bold">{value}</p>
            </div>
        </CardContent>
    </Card>
);

const EventStatsPage = () => {
    const { id } = useParams();
    const navigate = useNavigate();
    const [state, setState] = useState({ stats: null, loading: true, error: null });

    useEffect(() => {
        let cancelled = false;
        eventsApi
            .stats(id)
            .then((data) => {
                if (cancelled) return;
                setState({ stats: data?.stats ?? data, loading: false, error: null });
            })
            .catch((err) => {
                if (cancelled) return;
                setState({ stats: null, loading: false, error: err.message || 'Could not load stats.' });
            });
        return () => {
            cancelled = true;
        };
    }, [id]);

    const { stats, loading, error } = state;
    const byType = stats?.by_type ?? [];
    const maxSold = Math.max(1, ...byType.map((t) => t.sold ?? 0));

    return (
        <div className="mx-auto max-w-4xl">
            <Button variant="ghost" onClick={() => navigate(`/admin/events/${id}`)} className="mb-6">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Event
            </Button>

            <h1 className="mb-6 font-display text-3xl font-bold">Stats</h1>

            {loading && (
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
                    {[0, 1, 2].map((i) => (
                        <div key={i} className="h-24 animate-pulse rounded-xl bg-muted" />
                    ))}
                </div>
            )}

            {!loading && error && (
                <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-16 text-center">
                    <AlertCircle className="h-8 w-8 text-destructive" />
                    <p className="font-medium">{error}</p>
                </div>
            )}

            {!loading && !error && stats && (
                <>
                    <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
                        <StatTile icon={Ticket} label="Sold" value={stats.sold ?? 0} />
                        <StatTile icon={Coins} label="Revenue" value={formatMoney(stats.revenue_cents)} />
                        <StatTile icon={ShieldCheck} label="Admitted" value={stats.admitted ?? 0} />
                    </div>

                    <Card className="mt-6">
                        <CardHeader>
                            <CardTitle className="flex items-center gap-2 text-base">
                                <BarChart3 className="h-4 w-4" />
                                Sales by ticket type
                            </CardTitle>
                        </CardHeader>
                        <CardContent>
                            {byType.length === 0 ? (
                                <p className="text-sm text-muted-foreground">No ticket types sold yet.</p>
                            ) : (
                                <div className="space-y-4">
                                    {byType.map((t) => (
                                        <div key={t.ticket_type_id ?? t.name}>
                                            <div className="mb-1 flex justify-between text-sm">
                                                <span className="font-medium">{t.name}</span>
                                                <span className="text-muted-foreground">{t.sold ?? 0} sold</span>
                                            </div>
                                            <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
                                                <div
                                                    className="h-full rounded-full bg-primary transition-all"
                                                    style={{ width: `${Math.round(((t.sold ?? 0) / maxSold) * 100)}%` }}
                                                />
                                            </div>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </CardContent>
                    </Card>
                </>
            )}
        </div>
    );
};

export default EventStatsPage;
