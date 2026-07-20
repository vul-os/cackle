import React, { useState } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import {
    Calendar,
    MapPin,
    Image as ImageIcon,
    Info,
    Bold,
    Italic,
    List,
    ListOrdered,
    Quote,
    Link as LinkIcon,
    Eye,
    Heading1,
    Heading2,
    Save,
    Trash2,
    Coins,
} from 'lucide-react';
import DatePickerWithRange from '@/components/date-range-picker';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import ReactMarkdown from 'react-markdown';
import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
} from '@/components/ui/alert-dialog';

const CURRENCIES = ['ZAR', 'USD', 'EUR', 'GBP'];

const MarkdownToolbar = ({ onAction }) => (
    <div className="flex items-center gap-1 border-b bg-muted/40 p-1">
        {[
            { icon: Heading1, action: ['# ', ''], title: 'Heading 1' },
            { icon: Heading2, action: ['## ', ''], title: 'Heading 2' },
            { icon: Bold, action: ['**', '**'], title: 'Bold' },
            { icon: Italic, action: ['*', '*'], title: 'Italic' },
            { icon: List, action: ['\n- ', ''], title: 'List' },
            { icon: ListOrdered, action: ['\n1. ', ''], title: 'Numbered list' },
            { icon: Quote, action: ['\n> ', ''], title: 'Quote' },
            { icon: LinkIcon, action: ['[', '](url)'], title: 'Link' },
        ].map(({ icon: Icon, action, title }) => (
            <Button key={title} type="button" variant="ghost" size="sm" onClick={() => onAction(...action)} className="h-8 w-8 p-0" title={title}>
                <Icon className="h-4 w-4" />
            </Button>
        ))}
    </div>
);

const MarkdownEditor = ({ value, onChange, name, placeholder, minHeight = '200px', disabled }) => {
    const [activeTab, setActiveTab] = useState('write');

    const handleMarkdownAction = (prefix, suffix) => {
        const textarea = document.querySelector(`textarea[name="${name}"]`);
        if (!textarea) return;
        const start = textarea.selectionStart;
        const end = textarea.selectionEnd;
        const text = textarea.value;
        const newText = text.substring(0, start) + prefix + text.substring(start, end) + suffix + text.substring(end);
        onChange(newText);
        requestAnimationFrame(() => {
            textarea.focus();
            const cursor = start + prefix.length + (end - start) + suffix.length;
            textarea.setSelectionRange(cursor, cursor);
        });
    };

    return (
        <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
            <TabsList className="mb-2 grid w-full grid-cols-2">
                <TabsTrigger value="write" className="gap-2">
                    Write
                </TabsTrigger>
                <TabsTrigger value="preview" className="gap-2">
                    <Eye className="h-4 w-4" />
                    Preview
                </TabsTrigger>
            </TabsList>
            <TabsContent value="write" className="mt-0">
                <div className="rounded-md border">
                    <MarkdownToolbar onAction={handleMarkdownAction} />
                    <Textarea
                        name={name}
                        placeholder={placeholder}
                        value={value}
                        onChange={(e) => onChange(e.target.value)}
                        style={{ minHeight }}
                        className="resize-none rounded-none rounded-b-md border-0"
                        disabled={disabled}
                    />
                </div>
            </TabsContent>
            <TabsContent value="preview" className="mt-0">
                <div className="rounded-md border p-6" style={{ minHeight }}>
                    <div className="prose prose-sm dark:prose-invert max-w-none">
                        <ReactMarkdown>{value || '*Nothing to preview yet*'}</ReactMarkdown>
                    </div>
                </div>
            </TabsContent>
        </Tabs>
    );
};

const Section = ({ icon: Icon, title, children }) => (
    <div className="space-y-4 border-t border-border pt-6 first:border-t-0 first:pt-0">
        <div className="flex items-center gap-2 text-primary">
            <Icon className="h-4 w-4" />
            <h2 className="text-sm font-medium text-foreground">{title}</h2>
        </div>
        {children}
    </div>
);

export const EventDetailsCard = ({ editForm, handleInputChange, isSubmitting = false, hasChanges, onSave, onDelete }) => {
    const [showDeleteDialog, setShowDeleteDialog] = useState(false);

    const dateRange =
        editForm.starts_at && editForm.ends_at ? { from: new Date(editForm.starts_at), to: new Date(editForm.ends_at) } : undefined;

    const handleDateChange = (range) => {
        if (range?.from) handleInputChange('starts_at', range.from.toISOString());
        if (range?.to) handleInputChange('ends_at', range.to.toISOString());
    };

    return (
        <>
            <Card className="relative">
                <div className="absolute right-4 top-4 z-10 flex gap-2">
                    <Button variant="ghost" onClick={onSave} disabled={isSubmitting || !hasChanges} className="h-9 gap-2" title="Save changes">
                        <Save className={`h-4 w-4 ${isSubmitting ? 'animate-spin' : ''}`} />
                        Save
                    </Button>
                    <Button
                        variant="ghost"
                        onClick={() => setShowDeleteDialog(true)}
                        disabled={isSubmitting || !editForm.id}
                        className="h-9 w-9 p-0 text-destructive hover:bg-destructive/10 hover:text-destructive"
                        title="Delete event"
                    >
                        <Trash2 className="h-4 w-4" />
                    </Button>
                </div>

                <CardContent className="space-y-8 pt-6">
                    <Section icon={ImageIcon} title="Cover image">
                        <Input
                            placeholder="https://example.com/cover.jpg"
                            value={editForm.cover_image}
                            onChange={(e) => handleInputChange('cover_image', e.target.value)}
                            disabled={isSubmitting}
                        />
                        {editForm.cover_image && (
                            <div className="mt-2 aspect-[21/9] w-full overflow-hidden rounded-lg bg-muted">
                                <img src={editForm.cover_image} alt="Cover preview" className="h-full w-full object-cover" />
                            </div>
                        )}
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
                            <SelectTrigger className="w-48">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                {CURRENCIES.map((c) => (
                                    <SelectItem key={c} value={c}>
                                        {c}
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

            <AlertDialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Delete this event?</AlertDialogTitle>
                        <AlertDialogDescription>This cannot be undone. All ticket types and orders remain, but the event is removed.</AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction onClick={() => { setShowDeleteDialog(false); onDelete(); }} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
                            Delete
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </>
    );
};

export default EventDetailsCard;
