import React, { useEffect, useState } from 'react';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { ArrowLeft, ArrowRight } from 'lucide-react';
import DatePickerWithRange from '@/components/date-range-picker';
import { currencies as currenciesApi } from '@/lib/api';

// Fallback only: used if GET /api/currencies fails (offline, transient
// error). The primary source is the full ISO-4217 table below — Cackle
// has no privileged currency, so the picker shouldn't hardcode one either.
const FALLBACK_CURRENCIES = [
    { code: 'USD', name: 'United States Dollar' },
    { code: 'EUR', name: 'Euro' },
    { code: 'GBP', name: 'British Pound Sterling' },
    { code: 'ZAR', name: 'South African Rand' },
];

/** Hook: the full currency list from the backend, with a small offline fallback. */
function useCurrencyOptions() {
    const [options, setOptions] = useState(FALLBACK_CURRENCIES);
    useEffect(() => {
        let cancelled = false;
        currenciesApi
            .list()
            .then((data) => {
                if (cancelled) return;
                const list = data?.currencies;
                if (Array.isArray(list) && list.length > 0) setOptions(list);
            })
            .catch(() => {
                // Keep the fallback list — a picker with a few common
                // currencies beats a broken page.
            });
        return () => {
            cancelled = true;
        };
    }, []);
    return options;
}

const scheduleSchema = z
    .object({
        venue_name: z.string().trim().min(1, 'Venue name is required.').max(140),
        address: z.string().optional(),
        lat: z.string().optional(),
        lng: z.string().optional(),
        currency: z.string().min(1),
    })
    .refine((data) => data.lat === '' || data.lat === undefined || !Number.isNaN(Number(data.lat)), {
        message: 'Latitude must be a number.',
        path: ['lat'],
    })
    .refine((data) => data.lng === '' || data.lng === undefined || !Number.isNaN(Number(data.lng)), {
        message: 'Longitude must be a number.',
        path: ['lng'],
    });

const ScheduleVenueStep = ({ defaultValues, onSubmit, onBack, submitting }) => {
    const currencyOptions = useCurrencyOptions();
    const [dateRange, setDateRange] = useState(() =>
        defaultValues?.starts_at && defaultValues?.ends_at
            ? { from: new Date(defaultValues.starts_at), to: new Date(defaultValues.ends_at) }
            : undefined,
    );
    const [dateError, setDateError] = useState(null);

    const form = useForm({
        resolver: zodResolver(scheduleSchema),
        defaultValues: {
            venue_name: defaultValues?.venue_name || '',
            address: defaultValues?.address || '',
            lat: defaultValues?.lat != null ? String(defaultValues.lat) : '',
            lng: defaultValues?.lng != null ? String(defaultValues.lng) : '',
            // No hardcoded default currency — the organiser picks
            // explicitly (or it carries over from an event being edited).
            currency: defaultValues?.currency || '',
        },
    });

    const handleValidSubmit = (data) => {
        if (!dateRange?.from || !dateRange?.to) {
            setDateError('Pick a start and end date/time.');
            return;
        }
        if (new Date(dateRange.to) <= new Date(dateRange.from)) {
            setDateError('End must be after start.');
            return;
        }
        setDateError(null);
        onSubmit({
            ...data,
            starts_at: dateRange.from.toISOString(),
            ends_at: dateRange.to.toISOString(),
        });
    };

    return (
        <Form {...form}>
            <form onSubmit={form.handleSubmit(handleValidSubmit)} className="space-y-6">
                <div className="space-y-2">
                    <Label>When does it happen?</Label>
                    <DatePickerWithRange date={dateRange} setDate={setDateRange} className="w-full" />
                    {dateError && <p className="text-sm font-medium text-destructive">{dateError}</p>}
                </div>

                <FormField
                    control={form.control}
                    name="venue_name"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>Venue name</FormLabel>
                            <FormControl>
                                <Input {...field} placeholder="e.g. The Old Biscuit Mill" disabled={submitting} />
                            </FormControl>
                            <FormMessage />
                        </FormItem>
                    )}
                />

                <FormField
                    control={form.control}
                    name="address"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>Address</FormLabel>
                            <FormControl>
                                <Input {...field} placeholder="Street, suburb, city" disabled={submitting} />
                            </FormControl>
                            <FormMessage />
                        </FormItem>
                    )}
                />

                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                    <FormField
                        control={form.control}
                        name="lat"
                        render={({ field }) => (
                            <FormItem>
                                <FormLabel>Latitude (optional)</FormLabel>
                                <FormControl>
                                    <Input {...field} type="text" inputMode="decimal" placeholder="-33.9249" disabled={submitting} />
                                </FormControl>
                                <FormMessage />
                            </FormItem>
                        )}
                    />
                    <FormField
                        control={form.control}
                        name="lng"
                        render={({ field }) => (
                            <FormItem>
                                <FormLabel>Longitude (optional)</FormLabel>
                                <FormControl>
                                    <Input {...field} type="text" inputMode="decimal" placeholder="18.4241" disabled={submitting} />
                                </FormControl>
                                <FormMessage />
                            </FormItem>
                        )}
                    />
                </div>

                <FormField
                    control={form.control}
                    name="currency"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>Currency</FormLabel>
                            <Select value={field.value} onValueChange={field.onChange} disabled={submitting}>
                                <FormControl>
                                    <SelectTrigger className="w-64">
                                        <SelectValue placeholder="Select a currency" />
                                    </SelectTrigger>
                                </FormControl>
                                <SelectContent className="max-h-72">
                                    {currencyOptions.map((c) => (
                                        <SelectItem key={c.code} value={c.code}>
                                            {c.code} — {c.name}
                                        </SelectItem>
                                    ))}
                                </SelectContent>
                            </Select>
                            <FormMessage />
                        </FormItem>
                    )}
                />

                <div className="flex justify-between pt-2">
                    <Button type="button" variant="outline" onClick={onBack} disabled={submitting}>
                        <ArrowLeft className="mr-2 h-4 w-4" />
                        Back
                    </Button>
                    <Button type="submit" disabled={submitting}>
                        {submitting ? 'Saving…' : 'Continue'}
                        {!submitting && <ArrowRight className="ml-2 h-4 w-4" />}
                    </Button>
                </div>
            </form>
        </Form>
    );
};

export default ScheduleVenueStep;
