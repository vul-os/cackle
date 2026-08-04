import React, { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { ArrowLeft, ArrowRight, Plus, AlertTriangle } from 'lucide-react';
import { toast } from '@/components/ui/use-toast';
import { ticketTypes as ticketTypesApi } from '@/lib/api';
import type { TicketTypeInput } from '@/lib/api';
import type { TicketType } from '@/lib/api-types';
import TicketTypeForm from '@/pages/organizers/events/event/tickets/type-form';
import TicketTypeList from '@/pages/organizers/events/event/tickets/type-list';

interface TicketDialogState {
    open: boolean;
    editing: TicketType | null;
}

export interface TicketsStepProps {
    eventId?: string | null;
    currency: string;
    ticketTypes: TicketType[];
    onTicketTypesChange: (updater: (current: TicketType[]) => TicketType[]) => void;
    onBack: () => void;
    onSubmit: () => void;
    submitting?: boolean;
}

const TicketsStep = ({ eventId, currency, ticketTypes, onTicketTypesChange, onBack, onSubmit, submitting }: TicketsStepProps) => {
    const [dialog, setDialog] = useState<TicketDialogState>({ open: false, editing: null });
    const [isSaving, setIsSaving] = useState(false);

    const handleCreateOrUpdate = async (data: TicketTypeInput) => {
        setIsSaving(true);
        try {
            // Update is a full replace of every editable field (see
            // internal/events.TicketTypeInput's doc comment) — sort_order isn't
            // exposed in the form, so it must be carried through explicitly or
            // every edit would silently reset display order to 0.
            if (dialog.editing?.id) {
                const updated = await ticketTypesApi.update(dialog.editing.id, { ...data, sort_order: dialog.editing.sort_order ?? 0 });
                const tt = updated.ticket_type;
                onTicketTypesChange((current) => current.map((t) => (t.id === tt.id ? tt : t)));
                toast({ title: 'Updated', description: 'Ticket type updated.' });
            } else {
                const created = await ticketTypesApi.create(eventId ?? '', { ...data, sort_order: ticketTypes.length });
                const tt = created.ticket_type;
                onTicketTypesChange((current) => [...current, tt]);
                toast({ title: 'Added', description: 'Ticket type added.' });
            }
            setDialog({ open: false, editing: null });
        } catch (err) {
            const message = err instanceof Error ? err.message : undefined;
            toast({ title: 'Could not save', description: message, variant: 'destructive' });
        } finally {
            setIsSaving(false);
        }
    };

    const handleDelete = async (ticketTypeId: string) => {
        try {
            await ticketTypesApi.remove(ticketTypeId);
            onTicketTypesChange((current) => current.filter((t) => t.id !== ticketTypeId));
            toast({ title: 'Deleted', description: 'Ticket type removed.' });
        } catch (err) {
            const message = err instanceof Error ? err.message : undefined;
            toast({ title: 'Could not delete', description: message, variant: 'destructive' });
        }
    };

    return (
        <div className="space-y-6">
            <div className="flex flex-wrap items-start justify-between gap-3">
                <p className="max-w-md text-sm leading-relaxed text-muted-foreground">
                    A ticket type is what you&apos;re actually selling: a price, how many exist, when they go on sale, and
                    the most one order can take. All of it is enforced automatically at checkout — nobody can buy more than
                    you allow or buy after sales close.
                </p>
                <Button type="button" size="lg" onClick={() => setDialog({ open: true, editing: null })}>
                    <Plus className="mr-2 h-4 w-4" />
                    Add ticket type
                </Button>
            </div>

            <TicketTypeList ticketTypes={ticketTypes} currency={currency} onEdit={(tt) => setDialog({ open: true, editing: tt })} onDelete={handleDelete} />

            {ticketTypes.length === 0 && (
                <Alert variant="warning">
                    <AlertTriangle className="h-4 w-4" />
                    <AlertDescription>
                        No ticket types yet — you can come back to this step, but you won&apos;t be able to publish until at
                        least one exists.
                    </AlertDescription>
                </Alert>
            )}

            <div className="flex justify-between pt-2">
                <Button type="button" variant="outline" size="lg" onClick={onBack} disabled={submitting}>
                    <ArrowLeft className="mr-2 h-4 w-4" />
                    Back
                </Button>
                <Button type="button" size="lg" onClick={onSubmit} disabled={submitting}>
                    {submitting ? 'Saving…' : 'Continue'}
                    {!submitting && <ArrowRight className="ml-2 h-4 w-4" />}
                </Button>
            </div>

            <Dialog open={dialog.open} onOpenChange={(open) => setDialog({ open, editing: open ? dialog.editing : null })}>
                <DialogContent>
                    <DialogHeader>
                        <DialogTitle>{dialog.editing?.id ? 'Edit Ticket Type' : 'New Ticket Type'}</DialogTitle>
                    </DialogHeader>
                    <TicketTypeForm initialData={dialog.editing} currency={currency} onSubmit={handleCreateOrUpdate} isSubmitting={isSaving} />
                </DialogContent>
            </Dialog>
        </div>
    );
};

export default TicketsStep;
