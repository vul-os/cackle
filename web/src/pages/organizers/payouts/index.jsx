import React, { useCallback, useEffect, useState } from 'react';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Table, TableBody, TableCell, TableFooter, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { SkeletonList } from '@/components/ui/skeleton';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { toast } from '@/components/ui/use-toast';
import { Banknote, Landmark, ShieldCheck, Lock } from 'lucide-react';
import { useAuth } from '@/context/use-auth';
import { events as eventsApi, payoutsApi } from '@/lib/api';
import { formatMoney } from '@/lib/money';
import BankSelect from './bank-list';

// Payout status arrives as a backend enum (e.g. "no_sales", "unpaid",
// "paid"). Render human prose, never the raw snake_case token — a
// `capitalize` class alone leaves "No_sales" showing through.
const PAYOUT_STATUS_LABELS = {
    no_sales: 'No sales yet',
    unpaid: 'Unpaid',
    pending: 'Pending',
    processing: 'Processing',
    paid: 'Paid',
    failed: 'Failed',
};

function payoutStatusLabel(status) {
    if (!status) return 'Pending';
    return PAYOUT_STATUS_LABELS[status] ?? status.replace(/_/g, ' ').replace(/^\w/, (c) => c.toUpperCase());
}

const bankAccountSchema = z.object({
    bank_code: z.string().trim().min(1, 'Choose your bank.'),
    account_number: z
        .string()
        .trim()
        .min(4, 'Enter a valid account number.')
        .max(34, 'Enter a valid account number.'),
    account_name: z.string().trim().min(1, 'Enter the account holder name.'),
});

const PayoutsOverview = () => {
    const { activeOrg } = useAuth();
    const [state, setState] = useState({ rows: [], loading: true, error: null });

    const load = useCallback(async () => {
        if (!activeOrg?.id) {
            setState({ rows: [], loading: false, error: null });
            return;
        }
        setState((s) => ({ ...s, loading: true, error: null }));
        try {
            const data = await eventsApi.listForOrg(activeOrg.id);
            const eventList = Array.isArray(data) ? data : (data?.events ?? []);
            const results = await Promise.allSettled(eventList.map((ev) => payoutsApi.forEvent(ev.id)));
            const rows = eventList.map((ev, i) => {
                const r = results[i];
                const payout = r.status === 'fulfilled' ? (r.value?.payouts ?? r.value) : null;
                return { event: ev, payout, failed: r.status === 'rejected' };
            });
            setState({ rows, loading: false, error: null });
        } catch (err) {
            setState({ rows: [], loading: false, error: err.message || 'Could not load payouts.' });
        }
    }, [activeOrg?.id]);

    useEffect(() => {
        load();
    }, [load]);

    // Events can be denominated in different currencies (Cackle has no
    // privileged currency) — group totals per currency rather than
    // blending them into one meaningless number.
    const totalsByCurrency = state.rows.reduce((acc, r) => {
        if (!r.payout) return acc;
        const currency = r.event?.currency || r.payout.currency || '';
        const bucket = acc[currency] ?? { gross: 0, fees: 0, net: 0 };
        acc[currency] = {
            gross: bucket.gross + (r.payout.gross_minor ?? 0),
            fees: bucket.fees + (r.payout.fees_minor ?? 0),
            net: bucket.net + (r.payout.net_minor ?? 0),
        };
        return acc;
    }, {});

    return (
        <Card>
            <CardHeader>
                <CardTitle className="flex items-center gap-2 text-base">
                    <Banknote className="h-4 w-4" />
                    Payouts by event
                </CardTitle>
                <CardDescription>Gross sales, platform fees, and net payable per event.</CardDescription>
            </CardHeader>
            <CardContent>
                {state.loading ? (
                    <SkeletonList rows={3} />
                ) : state.error ? (
                    <ErrorState description={state.error} onRetry={load} />
                ) : state.rows.length === 0 ? (
                    <EmptyState icon={Banknote} title="No events yet" description="Payout figures show up once you have events selling tickets." />
                ) : (
                    <div className="overflow-x-auto">
                        <Table>
                            <TableHeader>
                                <TableRow>
                                    <TableHead>Event</TableHead>
                                    <TableHead className="text-right">Gross</TableHead>
                                    <TableHead className="text-right">Fees</TableHead>
                                    <TableHead className="text-right">Net</TableHead>
                                    <TableHead className="text-right">Status</TableHead>
                                </TableRow>
                            </TableHeader>
                            <TableBody>
                                {state.rows.map(({ event, payout, failed }) => (
                                    <TableRow key={event.id}>
                                        <TableCell className="max-w-[220px] truncate font-medium">{event.title}</TableCell>
                                        {failed || !payout ? (
                                            <TableCell colSpan={4} className="text-right text-sm text-muted-foreground">
                                                {failed ? 'Could not load' : 'No sales yet'}
                                            </TableCell>
                                        ) : (
                                            <>
                                                <TableCell className="text-right tabular-nums">{formatMoney(payout.gross_minor, event.currency)}</TableCell>
                                                <TableCell className="text-right tabular-nums text-muted-foreground">
                                                    -{formatMoney(payout.fees_minor, event.currency)}
                                                </TableCell>
                                                <TableCell className="text-right tabular-nums font-medium">{formatMoney(payout.net_minor, event.currency)}</TableCell>
                                                <TableCell className="text-right">
                                                    <Badge variant="outline">
                                                        {payoutStatusLabel(payout.status)}
                                                    </Badge>
                                                </TableCell>
                                            </>
                                        )}
                                    </TableRow>
                                ))}
                            </TableBody>
                            {Object.keys(totalsByCurrency).length > 0 && (
                                <TableFooter>
                                    {Object.entries(totalsByCurrency).map(([currency, t]) => (
                                        <TableRow key={currency || 'unknown'} className="font-semibold">
                                            <TableCell>Total{currency ? ` (${currency})` : ''}</TableCell>
                                            <TableCell className="text-right tabular-nums">{formatMoney(t.gross, currency)}</TableCell>
                                            <TableCell className="text-right tabular-nums text-muted-foreground">
                                                -{formatMoney(t.fees, currency)}
                                            </TableCell>
                                            <TableCell className="text-right tabular-nums">{formatMoney(t.net, currency)}</TableCell>
                                            <TableCell />
                                        </TableRow>
                                    ))}
                                </TableFooter>
                            )}
                        </Table>
                    </div>
                )}
            </CardContent>
        </Card>
    );
};

const BankAccountCard = () => {
    const { activeOrg } = useAuth();
    const isOwner = activeOrg?.role === 'owner';
    const [state, setState] = useState({ account: null, loading: true, error: null });
    const [saving, setSaving] = useState(false);
    const [editing, setEditing] = useState(false);

    const form = useForm({
        resolver: zodResolver(bankAccountSchema),
        defaultValues: { bank_code: '', account_number: '', account_name: '' },
    });

    const load = useCallback(async () => {
        if (!activeOrg?.id || !isOwner) {
            setState({ account: null, loading: false, error: null });
            return;
        }
        setState((s) => ({ ...s, loading: true, error: null }));
        try {
            const data = await payoutsApi.bankAccount(activeOrg.id);
            setState({ account: data?.bank_account ?? data, loading: false, error: null });
        } catch (err) {
            // A brand-new org with no bank account on file yet is a 404, not
            // an error worth alarming over — show the empty "set up" state.
            if (err.status === 404) {
                setState({ account: null, loading: false, error: null });
            } else {
                setState({ account: null, loading: false, error: err.message || 'Could not load payout bank details.' });
            }
        }
    }, [activeOrg?.id, isOwner]);

    useEffect(() => {
        load();
    }, [load]);

    const handleSave = async (data) => {
        setSaving(true);
        try {
            await payoutsApi.setBankAccount(activeOrg.id, data);
            toast({ title: 'Bank details saved' });
            form.reset({ bank_code: '', account_number: '', account_name: '' });
            setEditing(false);
            load();
        } catch (err) {
            toast({ title: 'Could not save bank details', description: err.message, variant: 'destructive' });
        } finally {
            setSaving(false);
        }
    };

    if (!isOwner) {
        return (
            <Card>
                <CardHeader>
                    <CardTitle className="flex items-center gap-2 text-base">
                        <Lock className="h-4 w-4" />
                        Payout bank account
                    </CardTitle>
                </CardHeader>
                <CardContent>
                    <p className="text-sm text-muted-foreground">Only the org owner can view or change payout bank details.</p>
                </CardContent>
            </Card>
        );
    }

    return (
        <Card>
            <CardHeader>
                <CardTitle className="flex items-center gap-2 text-base">
                    <Landmark className="h-4 w-4" />
                    Payout bank account
                </CardTitle>
                <CardDescription>Where payouts for this org are sent. Account numbers are masked once saved.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
                {state.loading ? (
                    <SkeletonList rows={1} />
                ) : state.error ? (
                    <ErrorState description={state.error} onRetry={load} />
                ) : (
                    <>
                        {state.account && !editing && (
                            <div className="flex items-center justify-between rounded-lg border border-border p-4">
                                <div className="flex items-center gap-3">
                                    <div className="rounded-full bg-primary/10 p-2 text-primary">
                                        <ShieldCheck className="h-4 w-4" />
                                    </div>
                                    <div>
                                        <p className="font-medium">{state.account.account_name}</p>
                                        <p className="text-sm text-muted-foreground">
                                            {state.account.bank_name || state.account.bank_code} · ····{state.account.account_number_last4}
                                        </p>
                                    </div>
                                </div>
                                <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
                                    Update
                                </Button>
                            </div>
                        )}

                        {(!state.account || editing) && (
                            <Form {...form}>
                                <form onSubmit={form.handleSubmit(handleSave)} className="space-y-4">
                                    <FormField
                                        control={form.control}
                                        name="bank_code"
                                        render={({ field }) => (
                                            <FormItem>
                                                <FormLabel>Bank</FormLabel>
                                                <FormControl>
                                                    <BankSelect value={field.value} onChange={field.onChange} disabled={saving} />
                                                </FormControl>
                                                <FormMessage />
                                            </FormItem>
                                        )}
                                    />
                                    <FormField
                                        control={form.control}
                                        name="account_number"
                                        render={({ field }) => (
                                            <FormItem>
                                                <FormLabel>Account number</FormLabel>
                                                <FormControl>
                                                    <Input {...field} inputMode="numeric" placeholder="e.g. 62812345678" disabled={saving} />
                                                </FormControl>
                                                <FormMessage />
                                            </FormItem>
                                        )}
                                    />
                                    <FormField
                                        control={form.control}
                                        name="account_name"
                                        render={({ field }) => (
                                            <FormItem>
                                                <FormLabel>Account holder name</FormLabel>
                                                <FormControl>
                                                    <Input {...field} placeholder="As it appears on the account" disabled={saving} />
                                                </FormControl>
                                                <FormMessage />
                                            </FormItem>
                                        )}
                                    />
                                    <div className="flex gap-2">
                                        <Button type="submit" disabled={saving}>
                                            {saving ? 'Saving…' : 'Save bank details'}
                                        </Button>
                                        {state.account && (
                                            <Button type="button" variant="outline" onClick={() => setEditing(false)} disabled={saving}>
                                                Cancel
                                            </Button>
                                        )}
                                    </div>
                                </form>
                            </Form>
                        )}
                    </>
                )}
            </CardContent>
        </Card>
    );
};

const PayoutsPage = () => {
    const { activeOrg } = useAuth();
    return (
        <div className="mx-auto max-w-4xl space-y-6">
            <div className="flex items-center gap-3">
                <Banknote className="h-8 w-8 text-primary" />
                <div>
                    <h1 className="font-display text-3xl font-bold">Payouts</h1>
                    {activeOrg && <p className="text-sm text-muted-foreground">{activeOrg.name}</p>}
                </div>
            </div>
            <PayoutsOverview />
            <BankAccountCard />
        </div>
    );
};

export default PayoutsPage;
