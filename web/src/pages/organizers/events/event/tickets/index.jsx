import React, { useCallback, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { ArrowLeft, Plus, AlertCircle } from 'lucide-react';
import { toast } from '@/components/ui/use-toast';
import { Spinner } from '@/components/ui/spinner';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import TicketTypeForm from './type-form';
import TicketTypeList from './type-list';
import { events as eventsApi, ticketTypes as ticketTypesApi } from '@/lib/api';

const EventTicketTypesPage = () => {
    const { id: eventId } = useParams();
    const navigate = useNavigate();
    const [state, setState] = useState({ event: null, ticketTypes: [], loading: true, error: null });
    const [dialog, setDialog] = useState({ open: false, editing: null });
    const [isSubmitting, setIsSubmitting] = useState(false);

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
            if (dialog.editing?.id) {
                await ticketTypesApi.update(dialog.editing.id, data);
                toast({ title: 'Updated', description: 'Ticket type updated.' });
            } else {
                await ticketTypesApi.create(eventId, data);
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

    const handleDelete = async (ticketTypeId) => {
        if (!window.confirm('Delete this ticket type?')) return;
        try {
            await ticketTypesApi.remove(ticketTypeId);
            toast({ title: 'Deleted', description: 'Ticket type removed.' });
            fetchAll();
        } catch (err) {
            toast({ title: 'Could not delete', description: err.message, variant: 'destructive' });
        }
    };

    if (state.loading) return <Spinner />;

    if (state.error) {
        return (
            <div className="mx-auto flex max-w-2xl flex-col items-center gap-3 py-16 text-center">
                <AlertCircle className="h-8 w-8 text-destructive" />
                <p className="font-medium">{state.error}</p>
                <Button variant="outline" onClick={fetchAll}>
                    Try again
                </Button>
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
                        onDelete={handleDelete}
                    />
                </CardContent>
            </Card>

            <Dialog open={dialog.open} onOpenChange={(open) => setDialog({ open, editing: open ? dialog.editing : null })}>
                <DialogContent>
                    <DialogHeader>
                        <DialogTitle>{dialog.editing?.id ? 'Edit Ticket Type' : 'New Ticket Type'}</DialogTitle>
                    </DialogHeader>
                    <TicketTypeForm initialData={dialog.editing} onSubmit={handleSubmit} isSubmitting={isSubmitting} />
                </DialogContent>
            </Dialog>
        </div>
    );
};

export default EventTicketTypesPage;
