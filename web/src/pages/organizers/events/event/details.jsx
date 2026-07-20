import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Button } from '@/components/ui/button';
import { Calendar, MapPin, Image as ImageIcon, Info, Save, Trash2, Coins, Tag, ArrowRight } from 'lucide-react';
import DatePickerWithRange from '@/components/date-range-picker';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { MarkdownEditor } from './markdown-editor';
import CategorySelect from './category-select';
import { images as imagesApi, currencies as currenciesApi } from '@/lib/api';

// Fallback only: used if GET /api/currencies fails (offline, transient
// error). The primary source is the full ISO-4217 table — Cackle has no
// privileged currency, so this picker shouldn't hardcode one either.
const FALLBACK_CURRENCIES = [
    { code: 'USD', name: 'United States Dollar' },
    { code: 'EUR', name: 'Euro' },
    { code: 'GBP', name: 'British Pound Sterling' },
    { code: 'ZAR', name: 'South African Rand' },
];

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
                // Keep the fallback list.
            });
        return () => {
            cancelled = true;
        };
    }, []);
    return options;
}

const Section = ({ icon: Icon, title, children }) => (
    <div className="space-y-4 border-t border-border pt-6 first:border-t-0 first:pt-0">
        <div className="flex items-center gap-2 text-primary">
            <Icon className="h-4 w-4" />
            <h2 className="text-sm font-medium text-foreground">{title}</h2>
        </div>
        {children}
    </div>
);

export const EventDetailsCard = ({ editForm, handleInputChange, isSubmitting = false, hasChanges, onSave, onDeleteRequest }) => {
    const navigate = useNavigate();
    const currencyOptions = useCurrencyOptions();

    const dateRange =
        editForm.starts_at && editForm.ends_at ? { from: new Date(editForm.starts_at), to: new Date(editForm.ends_at) } : undefined;

    const handleDateChange = (range) => {
        if (range?.from) handleInputChange('starts_at', range.from.toISOString());
        if (range?.to) handleInputChange('ends_at', range.to.toISOString());
    };

    const coverUrl = editForm.cover_image_id ? imagesApi.url(editForm.cover_image_id) : editForm.cover_image || null;

    return (
        <Card className="relative">
            <div className="absolute right-4 top-4 z-10 flex gap-2">
                <Button variant="ghost" onClick={onSave} disabled={isSubmitting || !hasChanges} className="h-9 gap-2" title="Save changes">
                    <Save className={`h-4 w-4 ${isSubmitting ? 'animate-spin' : ''}`} />
                    Save
                </Button>
                <Button
                    variant="ghost"
                    onClick={onDeleteRequest}
                    disabled={isSubmitting || !editForm.id}
                    className="h-9 w-9 p-0 text-destructive hover:bg-destructive/10 hover:text-destructive"
                    title="Delete event"
                >
                    <Trash2 className="h-4 w-4" />
                </Button>
            </div>

            <CardContent className="space-y-8 pt-6">
                <Section icon={ImageIcon} title="Cover image">
                    {coverUrl ? (
                        <div className="aspect-[21/9] w-full overflow-hidden rounded-lg bg-muted">
                            <img src={coverUrl} alt="Cover preview" className="h-full w-full object-cover" />
                        </div>
                    ) : (
                        <div className="flex aspect-[21/9] w-full items-center justify-center rounded-lg border border-dashed border-border bg-muted/40 text-sm text-muted-foreground">
                            No cover image yet
                        </div>
                    )}
                    <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="mt-3"
                        onClick={() => navigate(`/admin/events/${editForm.id}/images`)}
                        disabled={!editForm.id}
                    >
                        Manage images
                        <ArrowRight className="ml-2 h-4 w-4" />
                    </Button>
                </Section>

                <Section icon={Tag} title="Category">
                    <CategorySelect value={editForm.category} onChange={(value) => handleInputChange('category', value)} disabled={isSubmitting} />
                </Section>

                <Section icon={Calendar} title="Date & Time">
                    <DatePickerWithRange date={dateRange} setDate={handleDateChange} className="w-full" />
                </Section>

                <Section icon={MapPin} title="Location">
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                        <Input
                            placeholder="Venue name"
                            value={editForm.venue_name}
                            onChange={(e) => handleInputChange('venue_name', e.target.value)}
                            disabled={isSubmitting}
                        />
                        <Input
                            placeholder="Address"
                            value={editForm.address}
                            onChange={(e) => handleInputChange('address', e.target.value)}
                            disabled={isSubmitting}
                        />
                        <Input
                            type="number"
                            step="any"
                            placeholder="Latitude"
                            value={editForm.lat}
                            onChange={(e) => handleInputChange('lat', e.target.value)}
                            disabled={isSubmitting}
                        />
                        <Input
                            type="number"
                            step="any"
                            placeholder="Longitude"
                            value={editForm.lng}
                            onChange={(e) => handleInputChange('lng', e.target.value)}
                            disabled={isSubmitting}
                        />
                    </div>
                </Section>

                <Section icon={Coins} title="Currency">
                    <Select value={editForm.currency} onValueChange={(value) => handleInputChange('currency', value)} disabled={isSubmitting}>
                        <SelectTrigger className="w-64">
                            <SelectValue placeholder="Select a currency" />
                        </SelectTrigger>
                        <SelectContent className="max-h-72">
                            {currencyOptions.map((c) => (
                                <SelectItem key={c.code} value={c.code}>
                                    {c.code} — {c.name}
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                </Section>

                <Section icon={Info} title="Summary">
                    <Textarea
                        placeholder="One or two sentences shown in listings"
                        value={editForm.summary}
                        onChange={(e) => handleInputChange('summary', e.target.value)}
                        disabled={isSubmitting}
                    />
                </Section>

                <Section icon={Info} title="Description">
                    <MarkdownEditor
                        name="description"
                        value={editForm.description}
                        onChange={(value) => handleInputChange('description', value)}
                        placeholder="Write a compelling description using Markdown..."
                        minHeight="260px"
                        disabled={isSubmitting}
                    />
                </Section>
            </CardContent>
        </Card>
    );
};

export default EventDetailsCard;
