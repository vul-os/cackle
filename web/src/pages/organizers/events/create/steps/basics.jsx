import React from 'react';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { ArrowRight } from 'lucide-react';
import CategorySelect from '@/pages/organizers/events/event/category-select';
import { MarkdownEditor } from '@/pages/organizers/events/event/markdown-editor';

export const basicsSchema = z.object({
    title: z.string().trim().min(3, 'Give your event a title (at least 3 characters).').max(140),
    category: z.string().optional(),
    summary: z.string().max(240, 'Keep the summary under 240 characters.').optional(),
    description: z.string().optional(),
});

const BasicsStep = ({ defaultValues, onSubmit, submitting }) => {
    const form = useForm({
        resolver: zodResolver(basicsSchema),
        defaultValues: {
            title: defaultValues?.title || '',
            category: defaultValues?.category || '',
            summary: defaultValues?.summary || '',
            description: defaultValues?.description || '',
        },
    });

    return (
        <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
                <FormField
                    control={form.control}
                    name="title"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>Event title</FormLabel>
                            <FormControl>
                                <Input {...field} placeholder="e.g. Sunset Rooftop Sessions" autoFocus disabled={submitting} />
                            </FormControl>
                            <FormMessage />
                        </FormItem>
                    )}
                />

                <div className="space-y-2">
                    <Label>Category</Label>
                    <CategorySelect
                        value={form.watch('category')}
                        onChange={(value) => form.setValue('category', value, { shouldDirty: true })}
                        disabled={submitting}
                    />
                    <p className="text-sm text-muted-foreground">Helps buyers find you on the Browse page category filter.</p>
                </div>

                <FormField
                    control={form.control}
                    name="summary"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>Short summary</FormLabel>
                            <FormControl>
                                <Textarea {...field} placeholder="One or two sentences shown in listings" disabled={submitting} />
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
                            <FormLabel>Full description</FormLabel>
                            <FormControl>
                                <MarkdownEditor
                                    name="description"
                                    value={field.value}
                                    onChange={(value) => form.setValue('description', value, { shouldDirty: true })}
                                    placeholder="Write a compelling description using Markdown..."
                                    minHeight="220px"
                                    disabled={submitting}
                                />
                            </FormControl>
                            <FormMessage />
                        </FormItem>
                    )}
                />

                <div className="flex justify-end pt-2">
                    <Button type="submit" disabled={submitting}>
                        {submitting ? 'Saving…' : 'Continue'}
                        {!submitting && <ArrowRight className="ml-2 h-4 w-4" />}
                    </Button>
                </div>
            </form>
        </Form>
    );
};

export default BasicsStep;
