import React, { useState } from 'react';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { Button } from '@/components/ui/button';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage, FormDescription } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import DatePickerWithRange from '@/components/date-range-picker';
import { getExponent, decimalInputPattern, majorStringToMinor, minorToMajorString } from '@/lib/money';

const wholeNumber = /^\d+$/;

// Built per-currency (exponent-aware) rather than a single hardcoded
// "up to 2 decimals" pattern — JPY allows none, KWD allows three.
function priceSchemaFor(currency) {
    const exp = getExponent(currency);
    const pattern = exp === 0 ? /^\d+$/ : new RegExp(`^\\d+(\\.\\d{1,${exp}})?$`);
    const example = exp === 0 ? '150' : `150${'.' + '0'.repeat(exp)}`;
    return z
        .object({
            name: z.string().trim().min(1, 'Name is required.').max(80),
            description: z.string().max(500, 'Keep it under 500 characters.').optional(),
            price: z
                .string()
                .trim()
                .min(1, 'Enter a price (0 for free).')
                .regex(pattern, `Enter a valid amount, e.g. 150 or ${example}.`),
            quantity_total: z
                .string()
                .trim()
                .min(1, 'Enter how many are available.')
                .regex(wholeNumber, 'Whole numbers only.')
                .refine((v) => Number(v) >= 1, 'At least 1 ticket must be available.'),
            max_per_order: z
                .string()
                .trim()
                .min(1, 'Set a per-order limit.')
                .regex(wholeNumber, 'Whole numbers only.')
                .refine((v) => Number(v) >= 1, 'Must allow at least 1 per order.'),
        })
        .refine((data) => Number(data.max_per_order) <= Number(data.quantity_total), {
            message: 'Per-order limit can’t exceed total quantity.',
            path: ['max_per_order'],
        });
}

const TicketTypeForm = ({ initialData = null, currency, onSubmit, isSubmitting = false }) => {
    const [dateRange, setDateRange] = useState(() => {
        if (initialData?.sales_start && initialData?.sales_end) {
            return { from: new Date(initialData.sales_start), to: new Date(initialData.sales_end) };
        }
        return null;
    });
    const [dateError, setDateError] = useState(null);
    const alreadySold = initialData?.quantity_sold ?? 0;
    const inputPattern = decimalInputPattern(currency);

    const form = useForm({
        resolver: zodResolver(priceSchemaFor(currency)),
        defaultValues: {
            name: initialData?.name || '',
            description: initialData?.description || '',
            price: initialData?.price_minor !== undefined ? minorToMajorString(initialData.price_minor, currency) : '',
            quantity_total: initialData?.quantity_total?.toString() || '',
            max_per_order: initialData?.max_per_order?.toString() || '10',
        },
    });

    const handleSubmit = (formData) => {
        if (!dateRange?.from || !dateRange?.to) {
            setDateError('Pick a sales start and end date.');
            return;
        }
        if (new Date(dateRange.to) <= new Date(dateRange.from)) {
            setDateError('Sales end must be after sales start.');
            return;
        }
        if (alreadySold > Number(formData.quantity_total)) {
            setDateError(null);
            form.setError('quantity_total', {
                type: 'manual',
                message: `Can't be less than the ${alreadySold} already sold.`,
            });
            return;
        }
        setDateError(null);

        const priceMinor = majorStringToMinor(formData.price || '0', currency);
        if (priceMinor === null) {
            form.setError('price', { type: 'manual', message: 'Enter a valid price for this currency.' });
            return;
        }

        onSubmit({
            name: formData.name,
            description: formData.description,
            price_minor: priceMinor,
            quantity_total: parseInt(formData.quantity_total, 10) || 0,
            max_per_order: parseInt(formData.max_per_order, 10) || 10,
            sales_start: dateRange.from.toISOString(),
            sales_end: dateRange.to.toISOString(),
        });
    };

    return (
        <Form {...form}>
            <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
                <FormField
                    control={form.control}
                    name="name"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>Name</FormLabel>
                            <FormControl>
                                <Input {...field} placeholder="e.g. General Admission" disabled={isSubmitting} />
                            </FormControl>
                            <FormMessage />
                        </FormItem>
                    )}
                />

                <FormField
                    control={form.control}
                    name="description"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>Description</FormLabel>
                            <FormControl>
                                <Textarea {...field} placeholder="Optional details shown to buyers" disabled={isSubmitting} />
                            </FormControl>
                            <FormMessage />
                        </FormItem>
                    )}
                />

                <div className="grid grid-cols-2 gap-4">
                    <FormField
                        control={form.control}
                        name="price"
                        render={({ field }) => (
                            <FormItem>
                                <FormLabel>Price{currency ? ` (${currency})` : ''}</FormLabel>
                                <FormControl>
                                    <Input
                                        type="text"
                                        inputMode="decimal"
                                        placeholder={getExponent(currency) === 0 ? '0' : '0.00'}
                                        disabled={isSubmitting}
                                        {...field}
                                        onChange={(e) => {
                                            const value = e.target.value;
                                            if (value === '' || inputPattern.test(value)) field.onChange(value);
                                        }}
                                    />
                                </FormControl>
                                <FormDescription>0 makes it a free ticket.</FormDescription>
                                <FormMessage />
                            </FormItem>
                        )}
                    />

                    <FormField
                        control={form.control}
                        name="quantity_total"
                        render={({ field }) => (
                            <FormItem>
                                <FormLabel>Total quantity</FormLabel>
                                <FormControl>
                                    <Input
                                        type="text"
                                        inputMode="numeric"
                                        placeholder="0"
                                        disabled={isSubmitting}
                                        {...field}
                                        onChange={(e) => {
                                            const value = e.target.value;
                                            if (value === '' || /^\d+$/.test(value)) field.onChange(value);
                                        }}
                                    />
                                </FormControl>
                                {alreadySold > 0 && <FormDescription>{alreadySold} already sold.</FormDescription>}
                                <FormMessage />
                            </FormItem>
                        )}
                    />
                </div>

                <FormField
                    control={form.control}
                    name="max_per_order"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>Max per order</FormLabel>
                            <FormControl>
                                <Input
                                    type="text"
                                    inputMode="numeric"
                                    disabled={isSubmitting}
                                    {...field}
                                    onChange={(e) => {
                                        const value = e.target.value;
                                        if (value === '' || /^\d+$/.test(value)) field.onChange(value);
                                    }}
                                />
                            </FormControl>
                            <FormMessage />
                        </FormItem>
                    )}
                />

                <FormItem>
                    <FormLabel>Sale period</FormLabel>
                    <DatePickerWithRange date={dateRange} setDate={setDateRange} className="w-full" />
                    {dateError && <p className="text-sm font-medium text-destructive">{dateError}</p>}
                </FormItem>

                <Button type="submit" className="w-full" disabled={isSubmitting}>
                    {isSubmitting ? 'Saving...' : initialData?.id ? 'Update Ticket Type' : 'Create Ticket Type'}
                </Button>
            </form>
        </Form>
    );
};

export default TicketTypeForm;
