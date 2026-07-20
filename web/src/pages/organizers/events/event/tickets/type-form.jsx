import React, { useState } from 'react';
import { useForm } from 'react-hook-form';
import { Button } from '@/components/ui/button';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import DatePickerWithRange from '@/components/date-range-picker';

const TicketTypeForm = ({ initialData = null, onSubmit, isSubmitting = false }) => {
    const [dateRange, setDateRange] = useState(() => {
        if (initialData?.sales_start && initialData?.sales_end) {
            return { from: new Date(initialData.sales_start), to: new Date(initialData.sales_end) };
        }
        return null;
    });

    const form = useForm({
        defaultValues: {
            name: initialData?.name || '',
            description: initialData?.description || '',
            price: initialData?.price_cents !== undefined ? (initialData.price_cents / 100).toString() : '',
            quantity_total: initialData?.quantity_total?.toString() || '',
            max_per_order: initialData?.max_per_order?.toString() || '10',
        },
    });

    const handleSubmit = (formData) => {
        if (!dateRange?.from || !dateRange?.to) {
            form.setError('root', { type: 'manual', message: 'Please select a sales date range' });
            return;
        }

        onSubmit({
            name: formData.name,
            description: formData.description,
            price_cents: Math.round(parseFloat(formData.price || '0') * 100),
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
                                <Input {...field} required placeholder="e.g. General Admission" />
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
                                <Textarea {...field} placeholder="Optional details shown to buyers" />
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
                                <FormLabel>Price</FormLabel>
                                <FormControl>
                                    <Input
                                        type="number"
                                        step="0.01"
                                        min="0"
                                        required
                                        placeholder="0.00"
                                        {...field}
                                        onChange={(e) => {
                                            const value = e.target.value;
                                            if (value === '' || /^\d*\.?\d{0,2}$/.test(value)) field.onChange(value);
                                        }}
                                    />
                                </FormControl>
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
                                        type="number"
                                        min="0"
                                        required
                                        placeholder="0"
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
                </div>

                <FormField
                    control={form.control}
                    name="max_per_order"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>Max per order</FormLabel>
                            <FormControl>
                                <Input type="number" min="1" {...field} />
                            </FormControl>
                            <FormMessage />
                        </FormItem>
                    )}
                />

                <FormItem>
                    <FormLabel>Sale period</FormLabel>
                    <DatePickerWithRange date={dateRange} setDate={setDateRange} className="w-full" />
                </FormItem>

                {form.formState.errors.root && <p className="text-sm text-destructive">{form.formState.errors.root.message}</p>}

                <Button type="submit" className="w-full" disabled={isSubmitting}>
                    {isSubmitting ? 'Saving...' : initialData?.id ? 'Update Ticket Type' : 'Create Ticket Type'}
                </Button>
            </form>
        </Form>
    );
};

export default TicketTypeForm;
