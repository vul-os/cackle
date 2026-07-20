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

const EventTicketTypesPage = () => {
    const { id: eventId } = useParams();
    const navigate = useNavigate();
    const [state, setState] = useState({ event: null, ticketTypes: [], loading: true, error: null });
    const [dialog, setDialog] = useState({ open: false, editing: null });
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [deleteTarget, setDeleteTarget] = useState(null);
    const [isDeleting, setIsDeleting] = useState(false);

    const fetchAll = useCallback(async () => {
        setState((s) => ({ ...s, loading: true, error: null }));
        try {
            const [eventData, ticketTypesData] = await Promise.all([eventsApi.get(eventId), ticketTypesApi.list(eventId)]);
            const event = eventData?.event ?? eventData;
            const list = Array.isArray(ticketTypesData) ? ticketTypesData : (ticketTypesData?.ticket_types ?? []);
            setState({ event, ticketTypes: list, loading: false, error: null });
        } catch (err) {
            setState({ event: null, ticketTypes: [], loading: false, error: err.message || 'Could not load ticket types.' });
        }
    }, [eventId]);

    useEffect(() => {
        fetchAll();
    }, [fetchAll]);

    const handleSubmit = async (data) => {
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
                await ticketTypesApi.create(eventId, { ...data, sort_order: state.ticketTypes.length });
                toast({ title: 'Created', description: 'Ticket type created.' });
            }
            setDialog({ open: false, editing: null });
            fetchAll();
        } catch (err) {
            toast({ title: 'Could not save', description: err.message, variant: 'destructive' });
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
            toast({ title: 'Could not delete', description: err.message, variant: 'destructive' });
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
            <Button variant="ghost" onClick={() => navigate(`/admin/events/${eventId}`)} className="mb-6">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Event
            </Button>

            <Card>
                <CardHeader className="flex flex-row items-center justify-between">
                    <CardTitle>{state.event?.title ?? 'Ticket Types'}</CardTitle>
                    <Button onClick={() => setDialog({ open: true, editing: null })}>
                        <Plus className="mr-2 h-4 w-4" />
                        New Ticket Type
                    </Button>
                </CardHeader>
                <CardContent>
                    <TicketTypeList
                        ticketTypes={state.ticketTypes}
                        currency={state.event?.currency}
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
                        currency={state.event?.currency}
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
                            {deleteTarget?.quantity_sold > 0
                                ? `${deleteTarget.quantity_sold} of these have already been sold — those tickets remain valid, but no more can be issued.`
                                : 'This cannot be undone.'}
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel disabled={isDeleting}>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={(e) => {
                                e.preventDefault();
                                handleDelete();
                            }}
                            disabled={isDeleting}
                            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
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
