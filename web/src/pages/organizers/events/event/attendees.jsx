import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { format } from 'date-fns';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { SkeletonList } from '@/components/ui/skeleton';
import { ArrowLeft, Users, Ticket, Coins, ShieldCheck, Search, Download } from 'lucide-react';
import { events as eventsApi, ticketTypes as ticketTypesApi } from '@/lib/api';
import { formatMoney } from '@/lib/money';

const PAGE_SIZE = 50;

// Values the "Status" filter offers. "" means no filter. The rest map
// straight onto the backend's ?status= values (internal/store.AttendeeFilter):
// ticket status (valid/void/refunded) or gate status (admitted/not_admitted).
const STATUS_OPTIONS = [
    { value: 'all', label: 'All statuses' },
    { value: 'valid', label: 'Valid' },
    { value: 'admitted', label: 'Admitted' },
    { value: 'not_admitted', label: 'Not admitted yet' },
    { value: 'void', label: 'Void' },
    { value: 'refunded', label: 'Refunded' },
];

/** Hand-rolled CSV — no dependency. Wraps any field containing a comma,
 * quote or newline in quotes and escapes embedded quotes by doubling them. */
function toCsvField(value) {
    const s = value === undefined || value === null ? '' : String(value);
    if (/[",\n]/.test(s)) return `"${s.replace(/"/g, '""')}"`;
    return s;
}

function downloadCsv(filename, rows) {
    const csv = rows.map((row) => row.map(toCsvField).join(',')).join('\r\n');
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
}

function statusBadge(row) {
    if (row.status === 'void') return <Badge variant="destructive">Void</Badge>;
    if (row.status === 'refunded') return <Badge variant="destructive">Refunded</Badge>;
    if (row.admitted) return <Badge variant="secondary">Admitted</Badge>;
    return <Badge variant="outline">Valid</Badge>;
}

const StatTile = ({ icon: Icon, label, value }) => (
    <Card>
        <CardContent className="flex items-center gap-4 p-5">
            <div className="rounded-xl bg-primary/10 p-3 text-primary">
                <Icon className="h-5 w-5" />
            </div>
            <div className="min-w-0">
                <p className="text-sm text-muted-foreground">{label}</p>
                <p className="truncate text-2xl font-bold tabular-nums">{value}</p>
            </div>
        </CardContent>
    </Card>
);

const EventAttendeesPage = () => {
    const { id } = useParams();
    const navigate = useNavigate();

    // Event + ticket-type breakdown (aggregate summary, unchanged from before).
    const [summary, setSummary] = useState({ event: null, stats: null, priceByTypeId: {}, loading: true, error: null });

    // The real per-attendee roster.
    const [query, setQuery] = useState('');
    const [debouncedQuery, setDebouncedQuery] = useState('');
    const [status, setStatus] = useState('all');
    const [offset, setOffset] = useState(0);
    const [roster, setRoster] = useState({ attendees: [], total: 0, loading: true, error: null });

    const fetchSummary = useCallback(() => {
        setSummary((s) => ({ ...s, loading: true, error: null }));
        Promise.all([eventsApi.get(id), eventsApi.stats(id), ticketTypesApi.list(id)])
            .then(([eventData, statsData, ticketTypesData]) => {
                const types = Array.isArray(ticketTypesData) ? ticketTypesData : (ticketTypesData?.ticket_types ?? []);
                const priceByTypeId = {};
                for (const t of types) priceByTypeId[t.id] = t.price_minor;
                setSummary({
                    event: eventData?.event ?? eventData,
                    stats: statsData?.stats ?? statsData,
                    priceByTypeId,
                    loading: false,
                    error: null,
                });
            })
            .catch((err) => {
                setSummary({ event: null, stats: null, priceByTypeId: {}, loading: false, error: err.message || 'Could not load event summary.' });
            });
    }, [id]);

    useEffect(() => {
        fetchSummary();
    }, [fetchSummary]);

    // Debounce free-text search so every keystroke doesn't fire a request.
    useEffect(() => {
        const t = setTimeout(() => setDebouncedQuery(query.trim()), 300);
        return () => clearTimeout(t);
    }, [query]);

    // Any filter change resets to the first page.
    useEffect(() => {
        setOffset(0);
    }, [debouncedQuery, status]);

    const fetchRoster = useCallback(() => {
        setRoster((r) => ({ ...r, loading: true, error: null }));
        eventsApi
            .attendees(id, {
                q: debouncedQuery || undefined,
                status: status === 'all' ? undefined : status,
                limit: PAGE_SIZE,
                offset,
            })
            .then((data) => {
                setRoster({ attendees: data?.attendees ?? [], total: data?.total ?? 0, loading: false, error: null });
            })
            .catch((err) => {
                setRoster({ attendees: [], total: 0, loading: false, error: err.message || 'Could not load the attendee list.' });
            });
    }, [id, debouncedQuery, status, offset]);

    useEffect(() => {
        fetchRoster();
    }, [fetchRoster]);

    const { event, stats, priceByTypeId, loading: summaryLoading, error: summaryError } = summary;
    const currency = event?.currency || '';
    const byType = useMemo(() => stats?.by_type ?? [], [stats]);

    const { attendees, total: rosterTotal, loading: rosterLoading, error: rosterError } = roster;
    const page = Math.floor(offset / PAGE_SIZE) + 1;
    const pageCount = Math.max(1, Math.ceil(rosterTotal / PAGE_SIZE));

    const handleExportTypeCsv = () => {
        const rows = [
            ['Ticket type', 'Price', 'Sold', 'Capacity', 'Remaining', 'Revenue'],
            ...byType.map((t) => [
                t.name ?? '',
                formatMoney(priceByTypeId[t.ticket_type_id], currency),
                t.sold ?? 0,
                t.quantity_total ?? 0,
                Math.max(0, (t.quantity_total ?? 0) - (t.sold ?? 0)),
                formatMoney(t.revenue_minor, currency),
            ]),
        ];
        const base = event?.slug || id;
        downloadCsv(`${base}-ticket-type-summary.csv`, rows);
    };

    // Exports exactly what's currently loaded on screen (this page of the
    // roster, under the active search/filter) — not a silent full-table
    // dump the organiser didn't ask to download.
    const handleExportAttendeesCsv = () => {
        const rows = [
            ['Holder name', 'Ticket type', 'Serial', 'Order reference', 'Status', 'Issued', 'Admitted at'],
            ...attendees.map((a) => [
                a.holder_name ?? '',
                a.ticket_type_name ?? '',
                a.serial ?? '',
                a.order_id ?? '',
                a.status === 'valid' ? (a.admitted ? 'admitted' : 'valid') : a.status,
                a.issued_at ? format(new Date(a.issued_at), 'PPp') : '',
                a.admitted_at ? format(new Date(a.admitted_at), 'PPp') : '',
            ]),
        ];
        const base = event?.slug || id;
        downloadCsv(`${base}-attendees.csv`, rows);
    };

    return (
        <div className="mx-auto max-w-6xl">
            <Button variant="ghost" onClick={() => navigate(`/admin/events/${id}`)} className="mb-6">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Event
            </Button>

            <div className="mb-6 flex items-center gap-3">
                <Users className="h-8 w-8 text-primary" />
                <div className="min-w-0">
                    <h1 className="font-display text-3xl font-bold">Attendees</h1>
                    {!summaryLoading && !summaryError && event && (
                        <p className="truncate text-sm text-muted-foreground">
                            {event.title}
                            {event.venue_name ? ` · ${event.venue_name}` : ''}
                            {event.starts_at ? ` · ${format(new Date(event.starts_at), 'PPP')}` : ''}
                        </p>
                    )}
                </div>
            </div>

            {summaryLoading && (
                <div className="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-3">
                    {[0, 1, 2].map((i) => (
                        <div key={i} className="h-24 animate-pulse rounded-xl bg-muted" />
                    ))}
                </div>
            )}

            {!summaryLoading && summaryError && <ErrorState description={summaryError} onRetry={fetchSummary} />}

            {!summaryLoading && !summaryError && stats && (
                <>
                    <div className="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-3">
                        <StatTile icon={Ticket} label="Tickets sold" value={stats.sold ?? 0} />
                        <StatTile icon={ShieldCheck} label="Admitted at the gate" value={stats.admitted ?? 0} />
                        <StatTile icon={Coins} label="Revenue" value={formatMoney(stats.revenue_minor, currency)} />
                    </div>

                    <Card className="mb-6">
                        <CardHeader>
                            <CardTitle>Ticket type breakdown</CardTitle>
                            <CardDescription>Sales and capacity per ticket type.</CardDescription>
                        </CardHeader>
                        <CardContent>
                            {byType.length === 0 ? (
                                <EmptyState icon={Ticket} title="No ticket types yet" description="Once ticket types are on sale, their breakdown appears here." />
                            ) : (
                                <Table>
                                    <TableHeader>
                                        <TableRow>
                                            <TableHead>Ticket type</TableHead>
                                            <TableHead className="text-right">Price</TableHead>
                                            <TableHead className="text-right">Sold</TableHead>
                                            <TableHead className="text-right">Capacity</TableHead>
                                            <TableHead className="text-right">Remaining</TableHead>
                                            <TableHead className="text-right">Revenue</TableHead>
                                        </TableRow>
                                    </TableHeader>
                                    <TableBody>
                                        {byType.map((t) => {
                                            const remaining = Math.max(0, (t.quantity_total ?? 0) - (t.sold ?? 0));
                                            return (
                                                <TableRow key={t.ticket_type_id ?? t.name}>
                                                    <TableCell className="font-medium">
                                                        {t.name}
                                                        {remaining === 0 && (t.quantity_total ?? 0) > 0 && (
                                                            <Badge variant="destructive" className="ml-2 align-middle">
                                                                Sold out
                                                            </Badge>
                                                        )}
                                                    </TableCell>
                                                    <TableCell className="text-right tabular-nums">{formatMoney(priceByTypeId[t.ticket_type_id], currency)}</TableCell>
                                                    <TableCell className="text-right tabular-nums">{t.sold ?? 0}</TableCell>
                                                    <TableCell className="text-right tabular-nums">{t.quantity_total ?? 0}</TableCell>
                                                    <TableCell className="text-right tabular-nums">{remaining}</TableCell>
                                                    <TableCell className="text-right tabular-nums">{formatMoney(t.revenue_minor, currency)}</TableCell>
                                                </TableRow>
                                            );
                                        })}
                                    </TableBody>
                                </Table>
                            )}
                        </CardContent>
                        <div className="flex justify-end px-6 pb-4">
                            <Button variant="outline" size="sm" onClick={handleExportTypeCsv} disabled={byType.length === 0}>
                                <Download className="mr-2 h-4 w-4" />
                                Export ticket type summary
                            </Button>
                        </div>
                    </Card>
                </>
            )}

            <Card>
                <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                    <div>
                        <CardTitle>Attendee roster</CardTitle>
                        <CardDescription>
                            Every ticket holder for this event
                            {rosterTotal > 0 && !rosterLoading ? ` — ${rosterTotal} total` : ''}.
                        </CardDescription>
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                        <div className="relative">
                            <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                            <Input
                                placeholder="Search by name..."
                                value={query}
                                onChange={(e) => setQuery(e.target.value)}
                                className="h-9 w-48 pl-8"
                            />
                        </div>
                        <Select value={status} onValueChange={setStatus}>
                            <SelectTrigger className="h-9 w-44">
                                <SelectValue placeholder="Status" />
                            </SelectTrigger>
                            <SelectContent>
                                {STATUS_OPTIONS.map((opt) => (
                                    <SelectItem key={opt.value} value={opt.value}>
                                        {opt.label}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                        <Button variant="outline" size="sm" onClick={handleExportAttendeesCsv} disabled={attendees.length === 0}>
                            <Download className="mr-2 h-4 w-4" />
                            Export CSV
                        </Button>
                    </div>
                </CardHeader>
                <CardContent>
                    {rosterLoading && <SkeletonList rows={5} />}

                    {!rosterLoading && rosterError && <ErrorState description={rosterError} onRetry={fetchRoster} />}

                    {!rosterLoading && !rosterError && attendees.length === 0 && rosterTotal === 0 && (debouncedQuery || status !== 'all') && (
                        <EmptyState icon={Search} title="No matches" description="No attendee matches your search and filter." />
                    )}

                    {!rosterLoading && !rosterError && attendees.length === 0 && rosterTotal === 0 && !debouncedQuery && status === 'all' && (
                        <EmptyState icon={Users} title="No attendees yet" description="Once tickets are sold, everyone holding one shows up here." />
                    )}

                    {!rosterLoading && !rosterError && attendees.length > 0 && (
                        <>
                            <Table>
                                <TableHeader>
                                    <TableRow>
                                        <TableHead>Holder</TableHead>
                                        <TableHead>Ticket type</TableHead>
                                        <TableHead>Serial</TableHead>
                                        <TableHead>Order ref</TableHead>
                                        <TableHead>Issued</TableHead>
                                        <TableHead>Status</TableHead>
                                    </TableRow>
                                </TableHeader>
                                <TableBody>
                                    {attendees.map((a) => (
                                        <TableRow key={a.ticket_id}>
                                            <TableCell className="font-medium">{a.holder_name || 'Unnamed'}</TableCell>
                                            <TableCell>{a.ticket_type_name}</TableCell>
                                            <TableCell className="font-mono text-xs">{a.serial}</TableCell>
                                            <TableCell className="max-w-[10rem] truncate font-mono text-xs" title={a.order_id}>
                                                {a.order_id}
                                            </TableCell>
                                            <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                                                {a.issued_at ? format(new Date(a.issued_at), 'PP p') : '—'}
                                            </TableCell>
                                            <TableCell>
                                                <div className="flex flex-col gap-0.5">
                                                    {statusBadge(a)}
                                                    {a.admitted && a.admitted_at && (
                                                        <span className="text-xs text-muted-foreground">{format(new Date(a.admitted_at), 'PP p')}</span>
                                                    )}
                                                </div>
                                            </TableCell>
                                        </TableRow>
                                    ))}
                                </TableBody>
                            </Table>

                            {pageCount > 1 && (
                                <div className="mt-4 flex items-center justify-between">
                                    <p className="text-sm text-muted-foreground">
                                        Page {page} of {pageCount}
                                    </p>
                                    <div className="flex gap-2">
                                        <Button
                                            variant="outline"
                                            size="sm"
                                            disabled={offset === 0}
                                            onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
                                        >
                                            Previous
                                        </Button>
                                        <Button
                                            variant="outline"
                                            size="sm"
                                            disabled={offset + PAGE_SIZE >= rosterTotal}
                                            onClick={() => setOffset((o) => o + PAGE_SIZE)}
                                        >
                                            Next
                                        </Button>
                                    </div>
                                </div>
                            )}
                        </>
                    )}
                </CardContent>
            </Card>
        </div>
    );
};

export default EventAttendeesPage;
