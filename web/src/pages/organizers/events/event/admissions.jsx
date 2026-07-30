import React, { useCallback, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { format } from 'date-fns';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Accordion, AccordionItem, AccordionTrigger, AccordionContent } from '@/components/ui/accordion';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { SkeletonList } from '@/components/ui/skeleton';
import { ArrowLeft, Split, Ticket, Radio, Info, DoorOpen } from 'lucide-react';
import { events as eventsApi } from '@/lib/api';
import { summarize, orderClaims, gateLabel } from './admission-reconciliation';

// The report an organiser actually needs after an offline event: how many
// extra people got in, and at which gates — because two gates that could not
// see each other cannot be stopped from admitting the same ticket, and this
// screen exists to find that out AFTER THE FACT rather than pretend it can't
// happen. See lib/admission-reconciliation.js for the shaping logic (tested
// there) and docs/OFFLINE-GATES.md for the full story.
//
// Nothing on this page may read as prevention. The banner below states that
// once; every number under it is scoped by the same caveat the server itself
// sends back on every response, including an empty one.

function formatWhen(iso) {
    try {
        return format(new Date(iso), 'PP p');
    } catch {
        return iso;
    }
}

const StatTile = ({ icon: Icon, label, value, tone = 'primary' }) => (
    <Card>
        <CardContent className="flex items-center gap-4 p-5">
            <div
                className={`rounded-xl p-3 ${
                    tone === 'duplicate' ? 'bg-verdict-duplicate text-verdict-ink' : 'bg-primary/10 text-primary-emphasis'
                }`}
            >
                <Icon className="h-5 w-5" />
            </div>
            <div className="min-w-0">
                <p className="text-sm text-muted-foreground">{label}</p>
                <p className="tnum truncate text-2xl font-bold">{value}</p>
            </div>
        </CardContent>
    </Card>
);

/** One device's claim on one contested ticket. */
function ClaimRow({ claim }) {
    return (
        <div className="flex flex-col gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2.5 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline">{gateLabel(claim)}</Badge>
                <span className="font-mono text-xs text-muted-foreground">device {claim.device_id}</span>
            </div>
            <div className="flex flex-wrap items-center gap-2 text-sm">
                <span className="text-muted-foreground">{formatWhen(claim.scanned_at)}</span>
                <span className="font-semibold capitalize text-foreground">{claim.result}</span>
                {claim.extra ? (
                    <Badge className="bg-verdict-duplicate text-verdict-ink">Extra admission</Badge>
                ) : (
                    <Badge variant="success">First in</Badge>
                )}
                {claim.downgraded && (
                    <span className="text-xs text-muted-foreground">
                        stored server-side as <span className="font-semibold text-foreground">{claim.server_result}</span>
                    </span>
                )}
            </div>
        </div>
    );
}

const EventAdmissionsPage = () => {
    const { id } = useParams();
    const navigate = useNavigate();
    const [state, setState] = useState({ data: null, loading: true, error: null });

    const fetchConflicts = useCallback(() => {
        setState((s) => ({ ...s, loading: true, error: null }));
        eventsApi
            .admissionConflicts(id)
            .then((data) => setState({ data, loading: false, error: null }))
            .catch((err) =>
                setState({ data: null, loading: false, error: err.message || 'Could not load the admission reconciliation report.' }),
            );
    }, [id]);

    useEffect(() => {
        fetchConflicts();
    }, [fetchConflicts]);

    const { data, loading, error } = state;
    const summary = data ? summarize(data) : null;
    const conflicts = data?.conflicts ?? [];

    return (
        <div className="mx-auto max-w-5xl">
            <Button variant="ghost" onClick={() => navigate(`/admin/events/${id}`)} className="mb-6">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Event
            </Button>

            <div className="mb-6 flex items-center gap-3">
                <Split className="h-8 w-8 shrink-0 text-primary-emphasis" aria-hidden="true" />
                <div>
                    <h1 className="font-display text-3xl font-bold">Admission reconciliation</h1>
                    <p className="text-sm text-muted-foreground">Cross-gate double admissions this event's synced gates have reported.</p>
                </div>
            </div>

            {/* Always shown, loading or not: this governs how every number below
                must be read, and it is shown even before there is any data. */}
            <Card className="mb-6 bg-muted/30">
                <CardContent className="flex gap-3 p-5">
                    <Radio className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" aria-hidden="true" />
                    <div className="space-y-1.5 text-sm text-muted-foreground">
                        <p className="font-semibold text-foreground">Detected, never prevented.</p>
                        <p>
                            Every gate verifies a ticket's signature against its own key ring, entirely offline — that is
                            what lets it keep admitting people with no network at all. It also means two gates that
                            cannot see each other cannot be stopped from admitting the same ticket: neither one knows
                            the other exists. This report is what finds that out afterwards, once every gate's log has
                            synced. It is a record, not a guard, and no engineering change makes it one.
                        </p>
                        {summary && <p>{summary.caveat}</p>}
                    </div>
                </CardContent>
            </Card>

            {loading && <SkeletonList rows={4} />}
            {!loading && error && <ErrorState description={error} onRetry={fetchConflicts} />}

            {!loading && !error && summary && (
                <>
                    {!summary.complete && (
                        <Alert variant="warning" className="mb-6">
                            <Info className="h-4 w-4" aria-hidden="true" />
                            <AlertDescription>
                                This report is partial: at least one device's scan log has not synced, or a row could
                                not be replayed. Treat every number below as a floor, not a final count.
                            </AlertDescription>
                        </Alert>
                    )}

                    <div className="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2">
                        <StatTile icon={Split} label="Extra people admitted" value={summary.extraAdmissions} tone="duplicate" />
                        <StatTile icon={Ticket} label="Tickets with a conflict" value={summary.ticketsAffected} />
                    </div>

                    {summary.gates.length > 0 && (
                        <div className="mb-6 flex flex-wrap items-center gap-2">
                            <span className="flex items-center gap-1.5 text-sm font-medium text-muted-foreground">
                                <DoorOpen className="h-4 w-4 shrink-0" aria-hidden="true" />
                                Extra admissions by gate:
                            </span>
                            {summary.gates.map(({ gate, count }) => (
                                <Badge key={gate} className="bg-verdict-duplicate text-verdict-ink">
                                    {gate} · {count}
                                </Badge>
                            ))}
                        </div>
                    )}

                    <Card>
                        <CardHeader>
                            <CardTitle>Conflicting tickets</CardTitle>
                            <CardDescription>
                                One row per ticket that more than one gate device admitted. Expand a ticket to see every
                                device's own claim, in the order the scans happened.
                            </CardDescription>
                        </CardHeader>
                        <CardContent>
                            {conflicts.length === 0 ? (
                                <EmptyState
                                    icon={Split}
                                    title="No conflicts in what's synced so far"
                                    description="No ticket has more than one admission among the scans this server has received. A device that hasn't synced yet is not part of this picture."
                                />
                            ) : (
                                <Accordion type="multiple" className="w-full">
                                    {conflicts.map((conflict) => (
                                        <AccordionItem key={conflict.ticket_id} value={conflict.ticket_id}>
                                            <AccordionTrigger>
                                                <div className="flex flex-1 flex-wrap items-center justify-between gap-2 pr-2 text-left">
                                                    <span className="font-mono text-xs text-muted-foreground">{conflict.ticket_id}</span>
                                                    <span className="flex items-center gap-2">
                                                        <Badge variant="outline">{conflict.devices} devices</Badge>
                                                        <Badge className="bg-verdict-duplicate text-verdict-ink">
                                                            +{conflict.extra_admissions} extra
                                                        </Badge>
                                                    </span>
                                                </div>
                                            </AccordionTrigger>
                                            <AccordionContent>
                                                <div className="space-y-2">
                                                    {orderClaims(conflict).map((claim, i) => (
                                                        <ClaimRow key={`${claim.device_id}-${claim.scanned_at}-${i}`} claim={claim} />
                                                    ))}
                                                </div>
                                            </AccordionContent>
                                        </AccordionItem>
                                    ))}
                                </Accordion>
                            )}
                        </CardContent>
                    </Card>
                </>
            )}
        </div>
    );
};

export default EventAdmissionsPage;
