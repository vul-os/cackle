import React, { useCallback, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { ArrowLeft, Plus } from 'lucide-react';
import { toast } from '@/components/ui/use-toast';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
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
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { SkeletonList } from '@/components/ui/skeleton';
import { ErrorState } from '@/components/ui/error-state';
import TicketTypeForm from './type-form';
import TicketTypeList from './type-list';
import { events as eventsApi, ticketTypes as ticketTypesApi } from '@/lib/api';
import type { TicketTypeInput } from '@/lib/api';
import type { CackleEvent, TicketType } from '@/lib/api-types';

interface EventTicketTypesState {
    event: CackleEvent | null;
    ticketTypes: TicketType[];
    loading: boolean;
    error: string | null;
}

interface DialogState {
    open: boolean;
    editing: TicketType | null;
}

/** Either a real ticket type, or the synthetic {id} placeholder used when the id being deleted somehow isn't in state.ticketTypes. */
type DeleteTarget = Partial<TicketType> & { id: string };

const EventTicketTypesPage = () => {
    const { id: eventId } = useParams();
    const navigate = useNavigate();
    const [state, setState] = useState<EventTicketTypesState>({ event: null, ticketTypes: [], loading: true, error: null });
    const [dialog, setDialog] = useState<DialogState>({ open: false, editing: null });
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null);
    const [isDeleting, setIsDeleting] = useState(false);

    const fetchAll = useCallback(async () => {
        setState((s) => ({ ...s, loading: true, error: null }));
        try {
            const [eventData, ticketTypesData] = await Promise.all([eventsApi.get(eventId ?? ''), ticketTypesApi.list(eventId ?? '')]);
            setState({ event: eventData.event, ticketTypes: ticketTypesData.ticket_types ?? [], loading: false, error: null });
        } catch (err) {
            const message = err instanceof Error ? err.message : 'Could not load ticket types.';
            setState({ event: null, ticketTypes: [], loading: false, error: message });
        }
    }, [eventId]);

    useEffect(() => {
        fetchAll();
    }, [fetchAll]);

    const handleSubmit = async (data: TicketTypeInput) => {
        setIsSubmitting(true);
        try {
            // Update is a full replace of every editable field (see
            // internal/events.TicketTypeInput's doc comment) — sort_order isn't
            // exposed in the form, so it must be carried through explicitly or
            // every edit would silently reset display order to 0.
            if (dialog.editing?.id) {
                await ticketTypesApi.update(dialog.editing.id, { ...data, sort_order: dialog.editing.sort_order ?? 0 });
                toast({ title: 'Updated', description: 'Ticket type updated.' });
            } else {
                await ticketTypesApi.create(eventId ?? '', { ...data, sort_order: state.ticketTypes.length });
                toast({ title: 'Created', description: 'Ticket type created.' });
            }
            setDialog({ open: false, editing: null });
            fetchAll();
        } catch (err) {
            const message = err instanceof Error ? err.message : undefined;
            toast({ title: 'Could not save', description: message, variant: 'destructive' });
        } finally {
            setIsSubmitting(false);
        }
    };

    const handleDelete = async () => {
        if (!deleteTarget) return;
        setIsDeleting(true);
        try {
            await ticketTypesApi.remove(deleteTarget.id);
            toast({ title: 'Deleted', description: 'Ticket type removed.' });
            setDeleteTarget(null);
            fetchAll();
        } catch (err) {
            const message = err instanceof Error ? err.message : undefined;
            toast({ title: 'Could not delete', description: message, variant: 'destructive' });
        } finally {
            setIsDeleting(false);
        }
    };

    if (state.loading) {
        return (
            <div className="mx-auto max-w-4xl">
                <SkeletonList rows={4} />
            </div>
        );
    }

    if (state.error) {
        return (
            <div className="mx-auto max-w-2xl py-8">
                <ErrorState description={state.error} onRetry={fetchAll} />
            </div>
        );
    }

    return (
        <div className="mx-auto max-w-4xl">
            <Button variant="ghost" onClick={() => navigate(`/admin/events/${eventId}`)} className="mb-6 h-11">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Event
            </Button>

            <Card>
                <CardHeader className="flex flex-row items-center justify-between">
                    <CardTitle>{state.event?.title ?? 'Ticket Types'}</CardTitle>
                    <Button size="lg" onClick={() => setDialog({ open: true, editing: null })}>
                        <Plus className="mr-2 h-4 w-4" />
                        New Ticket Type
                    </Button>
                </CardHeader>
                <CardContent>
                    <TicketTypeList
                        ticketTypes={state.ticketTypes}
                        currency={state.event?.currency ?? ''}
                        onEdit={(tt) => setDialog({ open: true, editing: tt })}
                        onDelete={(id) => setDeleteTarget(state.ticketTypes.find((t) => t.id === id) ?? { id })}
                    />
                </CardContent>
            </Card>

            <Dialog open={dialog.open} onOpenChange={(open) => setDialog({ open, editing: open ? dialog.editing : null })}>
                <DialogContent>
                    <DialogHeader>
                        <DialogTitle>{dialog.editing?.id ? 'Edit Ticket Type' : 'New Ticket Type'}</DialogTitle>
                    </DialogHeader>
                    <TicketTypeForm
                        initialData={dialog.editing}
                        currency={state.event?.currency ?? ''}
                        onSubmit={handleSubmit}
                        isSubmitting={isSubmitting}
                    />
                </DialogContent>
            </Dialog>

            <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !isDeleting && !open && setDeleteTarget(null)}>
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Delete “{deleteTarget?.name || 'this ticket type'}”?</AlertDialogTitle>
                        <AlertDialogDescription>
                            {deleteTarget?.quantity_sold && deleteTarget.quantity_sold > 0
                                ? `${deleteTarget.quantity_sold} of these have already been sold — those tickets remain valid, but no more can be issued.`
                                : 'This cannot be undone.'}
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel disabled={isDeleting} className="h-11">
                            Cancel
                        </AlertDialogCancel>
                        <AlertDialogAction
                            onClick={(e) => {
                                e.preventDefault();
                                handleDelete();
                            }}
                            disabled={isDeleting}
                            className="h-11 bg-destructive text-destructive-foreground hover:bg-destructive/90"
                        >
                            Delete
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </div>
    );
};

export default EventTicketTypesPage;
