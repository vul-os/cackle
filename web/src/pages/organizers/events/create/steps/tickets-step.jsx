import React, { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { ArrowLeft, ArrowRight, Plus, AlertTriangle } from 'lucide-react';
import { toast } from '@/components/ui/use-toast';
import { ticketTypes as ticketTypesApi } from '@/lib/api';
import TicketTypeForm from '@/pages/organizers/events/event/tickets/type-form';
import TicketTypeList from '@/pages/organizers/events/event/tickets/type-list';

const TicketsStep = ({ eventId, currency, ticketTypes, onTicketTypesChange, onBack, onSubmit, submitting }) => {
    const [dialog, setDialog] = useState({ open: false, editing: null });
    const [isSaving, setIsSaving] = useState(false);

    const handleCreateOrUpdate = async (data) => {
        setIsSaving(true);
        try {
            // Update is a full replace of every editable field (see
            // internal/events.TicketTypeInput's doc comment) — sort_order isn't
            // exposed in the form, so it must be carried through explicitly or
            // every edit would silently reset display order to 0.
            if (dialog.editing?.id) {
                const updated = await ticketTypesApi.update(dialog.editing.id, { ...data, sort_order: dialog.editing.sort_order ?? 0 });
                const tt = updated?.ticket_type ?? updated;
                onTicketTypesChange((current) => current.map((t) => (t.id === tt.id ? tt : t)));
                toast({ title: 'Updated', description: 'Ticket type updated.' });
            } else {
                const created = await ticketTypesApi.create(eventId, { ...data, sort_order: ticketTypes.length });
                const tt = created?.ticket_type ?? created;
                onTicketTypesChange((current) => [...current, tt]);
                toast({ title: 'Added', description: 'Ticket type added.' });
            }
            setDialog({ open: false, editing: null });
        } catch (err) {
            toast({ title: 'Could not save', description: err.message, variant: 'destructive' });
        } finally {
            setIsSaving(false);
        }
    };

    const handleDelete = async (ticketTypeId) => {
        try {
            await ticketTypesApi.remove(ticketTypeId);
            onTicketTypesChange((current) => current.filter((t) => t.id !== ticketTypeId));
            toast({ title: 'Deleted', description: 'Ticket type removed.' });
        } catch (err) {
            toast({ title: 'Could not delete', description: err.message, variant: 'destructive' });
        }
    };

    return (
        <div className="space-y-6">
            <div className="flex items-center justify-between">
                <p className="text-sm text-muted-foreground">Add at least one ticket type so buyers have something to purchase.</p>
                <Button type="button" size="sm" onClick={() => setDialog({ open: true, editing: null })}>
                    <Plus className="mr-2 h-4 w-4" />
                    Add ticket type
                </Button>
            </div>

            <TicketTypeList ticketTypes={ticketTypes} currency={currency} onEdit={(tt) => setDialog({ open: true, editing: tt })} onDelete={handleDelete} />

            {ticketTypes.length === 0 && (
                <div className="flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-700 dark:text-amber-400">
                    <AlertTriangle className="h-4 w-4 shrink-0" />
                    No ticket types yet — you can add them later, but you won't be able to publish until you do.
                </div>
            )}

            <div className="flex justify-between pt-2">
                <Button type="button" variant="outline" onClick={onBack} disabled={submitting}>
                    <ArrowLeft className="mr-2 h-4 w-4" />
                    Back
                </Button>
                <Button type="button" onClick={onSubmit} disabled={submitting}>
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
