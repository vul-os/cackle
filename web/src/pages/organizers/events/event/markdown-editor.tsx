import React, { useState } from 'react';
import { Textarea } from '@/components/ui/textarea';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Button } from '@/components/ui/button';
import { Bold, Italic, List, ListOrdered, Quote, Link as LinkIcon, Eye, Heading1, Heading2, type LucideIcon } from 'lucide-react';
import ReactMarkdown from 'react-markdown';

// Shared write/preview Markdown field for event descriptions — used by both
// the flat event editor (details.tsx) and the "basics" step of the create
// wizard, so the two don't drift into two slightly-different editors.

interface ToolbarItem {
    icon: LucideIcon;
    action: [string, string];
    title: string;
}

const TOOLBAR_ITEMS: ToolbarItem[] = [
    { icon: Heading1, action: ['# ', ''], title: 'Heading 1' },
    { icon: Heading2, action: ['## ', ''], title: 'Heading 2' },
    { icon: Bold, action: ['**', '**'], title: 'Bold' },
    { icon: Italic, action: ['*', '*'], title: 'Italic' },
    { icon: List, action: ['\n- ', ''], title: 'List' },
    { icon: ListOrdered, action: ['\n1. ', ''], title: 'Numbered list' },
    { icon: Quote, action: ['\n> ', ''], title: 'Quote' },
    { icon: LinkIcon, action: ['[', '](url)'], title: 'Link' },
];

// Eight 44px buttons in a row do not fit a 390px phone (8 * 44px + 7 * 4px
// gap = 380px against ~340px of usable card width) — rather than shrink
// them below a real touch target, or let them wrap and eat vertical space
// from the textarea below, the row scrolls horizontally. `overflow-x-auto`
// on a container whose content genuinely exceeds it is the one thing that
// legitimately forgives an "overflowing" child.
const MarkdownToolbar = ({ onAction }: { onAction: (prefix: string, suffix: string) => void }) => (
    <div className="flex items-center gap-1 overflow-x-auto border-b border-border bg-muted/40 p-1">
        {TOOLBAR_ITEMS.map(({ icon: Icon, action, title }) => (
            <Button key={title} type="button" variant="ghost" onClick={() => onAction(...action)} className="h-11 w-11 shrink-0 p-0" title={title}>
                <Icon className="h-4 w-4" />
                <span className="sr-only">{title}</span>
            </Button>
        ))}
    </div>
);

export interface MarkdownEditorProps {
    value: string;
    onChange: (value: string) => void;
    name: string;
    placeholder?: string | undefined;
    minHeight?: string | undefined;
    disabled?: boolean | undefined;
}

export const MarkdownEditor = ({ value, onChange, name, placeholder, minHeight = '200px', disabled }: MarkdownEditorProps) => {
    const [activeTab, setActiveTab] = useState('write');

    const handleMarkdownAction = (prefix: string, suffix: string) => {
        const textarea = document.querySelector<HTMLTextAreaElement>(`textarea[name="${name}"]`);
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
                <div className="rounded-md border border-border">
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
                <div className="rounded-md border border-border p-6" style={{ minHeight }}>
                    <div className="prose prose-sm max-w-none dark:prose-invert">
                        <ReactMarkdown>{value || '*Nothing to preview yet*'}</ReactMarkdown>
                    </div>
                </div>
            </TabsContent>
        </Tabs>
    );
};

export default MarkdownEditor;
