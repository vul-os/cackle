"use client";

import React, { useEffect, useState, useCallback, memo } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { supabase } from '@/services/supabaseClient';
import { Button } from '@/components/ui/button';
import { ArrowLeft, Plus } from 'lucide-react';
import { useToast } from "@/components/ui/use-toast";
import { Spinner } from '@/components/ui/spinner';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { ticketTypeService } from './service';
import TicketTypeForm from './type-form';
import TicketTypeList from './type-list';
import { addDays } from 'date-fns';

const EventHeader = memo(({ title, onCreateClick }) => (
  <div className="flex items-center justify-between mb-6">
    <h1 className="text-2xl font-bold">{title} - Ticket Types</h1>
    <Button onClick={onCreateClick}>
      <Plus className="h-4 w-4 mr-2" />
      Create Ticket Type
    </Button>
  </div>
));

EventHeader.displayName = 'EventHeader';

const BackButton = memo(({ eventId, navigate }) => (
  <Button
    variant="ghost"
    onClick={() => navigate(`/admin/events/${eventId}`)}
    className="mb-4 hover:bg-gray-100"
  >
    <ArrowLeft className="h-4 w-4 mr-2" />
    Back to Event
  </Button>
));

BackButton.displayName = 'BackButton';

const EventTicketTypes = () => {
  const { id: eventId } = useParams();
  const navigate = useNavigate();
  const { toast } = useToast();
  const [state, setState] = useState({
    event: null,
    ticketTypes: [],
    loading: true,
    isDialogOpen: false,
    editingTicketType: null,
    isSubmitting: false
  });

  const fetchEventAndTicketTypes = useCallback(async () => {
    try {
      setState(prev => ({ ...prev, loading: true }));
      
      const [{ data: eventData, error: eventError }, ticketTypesData] = await Promise.all([
        supabase.from('events').select('*').eq('id', eventId).single(),
        ticketTypeService.fetchTicketTypes(eventId)
      ]);

      if (eventError) throw eventError;

      setState(prev => ({
        ...prev,
        event: eventData,
        ticketTypes: ticketTypesData,
        loading: false
      }));
    } catch (error) {
      console.error('Error fetching data:', error);
      toast({
        title: "Error",
        description: "Failed to fetch event and ticket types",
        variant: "destructive"
      });
      setState(prev => ({ ...prev, loading: false }));
    }
  }, [eventId, toast]);

  useEffect(() => {
    fetchEventAndTicketTypes();
  }, [fetchEventAndTicketTypes]);

  const getInitialTicketDates = useCallback((ticketType = null) => {
    if (ticketType?.sale_start_time && ticketType?.sale_end_time) {
      return {
        sale_start_time: new Date(ticketType.sale_start_time).toISOString(),
        sale_end_time: new Date(ticketType.sale_end_time).toISOString()
      };
    }

    const now = new Date();
    now.setHours(0, 0, 0, 0);

    if (state.event?.start_date && state.event?.end_date) {
      const eventStart = new Date(state.event.start_date);
      const eventEnd = new Date(state.event.end_date);
      const saleStart = eventStart > now ? now : now;
      const defaultEnd = addDays(now, 5);
      const saleEnd = eventEnd < defaultEnd ? eventEnd : defaultEnd;

      return {
        sale_start_time: saleStart.toISOString(),
        sale_end_time: saleEnd.toISOString()
      };
    }

    return {
      sale_start_time: now.toISOString(),
      sale_end_time: addDays(now, 5).toISOString()
    };
  }, [state.event]);

  const handleSubmit = useCallback(async (formData) => {
    try {
      setState(prev => ({ ...prev, isSubmitting: true }));
      
      const ticketTypeData = {
        event_id: eventId,
        organization_id: state.event.organization_id,
        name: formData.name,
        description: formData.description,
        price: parseFloat(formData.price),
        quantity_total: parseInt(formData.quantity_total),
        sale_start_time: formData.sale_start_time,
        sale_end_time: formData.sale_end_time,
      };

      if (state.editingTicketType) {
        await ticketTypeService.updateTicketType(state.editingTicketType.id, ticketTypeData);
        toast({
          title: "Success",
          description: "Ticket type updated successfully",
        });
      } else {
        await ticketTypeService.createTicketType(ticketTypeData);
        toast({
          title: "Success",
          description: "Ticket type created successfully",
        });
      }

      setState(prev => ({
        ...prev,
        isDialogOpen: false,
        editingTicketType: null,
        isSubmitting: false
      }));
      fetchEventAndTicketTypes();
    } catch (error) {
      console.error('Error saving ticket type:', error);
      toast({
        title: "Error",
        description: "Failed to save ticket type",
        variant: "destructive"
      });
      setState(prev => ({ ...prev, isSubmitting: false }));
    }
  }, [eventId, state.event, state.editingTicketType, toast, fetchEventAndTicketTypes]);

  const handleDelete = useCallback(async (ticketTypeId) => {
    if (!window.confirm('Are you sure you want to delete this ticket type?')) return;

    try {
      await ticketTypeService.deleteTicketType(ticketTypeId);
      toast({
        title: "Success",
        description: "Ticket type deleted successfully",
      });
      fetchEventAndTicketTypes();
    } catch (error) {
      console.error('Error deleting ticket type:', error);
      toast({
        title: "Error",
        description: "Failed to delete ticket type",
        variant: "destructive"
      });
    }
  }, [toast, fetchEventAndTicketTypes]);

  const handleCreateTicketType = useCallback(() => {
    setState(prev => ({
      ...prev,
      editingTicketType: {
        ...getInitialTicketDates()
      },
      isDialogOpen: true
    }));
  }, [getInitialTicketDates]);

  const handleEditTicketType = useCallback((ticketType) => {
    setState(prev => ({
      ...prev,
      editingTicketType: {
        ...ticketType,
        ...getInitialTicketDates(ticketType)
      },
      isDialogOpen: true
    }));
  }, [getInitialTicketDates]);

  const handleDialogOpenChange = useCallback((open) => {
    setState(prev => ({ ...prev, isDialogOpen: open }));
  }, []);

  if (state.loading) return <Spinner />;
  if (!state.event) return <div>Event not found</div>;

  return (
    <div className="min-h-screen bg-white p-4 md:p-8">
      <div className="max-w-4xl mx-auto">
        <div className="mb-8">
          <BackButton eventId={eventId} navigate={navigate} />

          <EventHeader 
            title={state.event.title} 
            onCreateClick={handleCreateTicketType}
          />

          <Dialog open={state.isDialogOpen} onOpenChange={handleDialogOpenChange}>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>
                  {state.editingTicketType?.id ? 'Edit Ticket Type' : 'Create New Ticket Type'}
                </DialogTitle>
              </DialogHeader>
              <TicketTypeForm
                initialData={state.editingTicketType}
                onSubmit={handleSubmit}
                isSubmitting={state.isSubmitting}
              />
            </DialogContent>
          </Dialog>

          <TicketTypeList
            ticketTypes={state.ticketTypes}
            onEdit={handleEditTicketType}
            onDelete={handleDelete}
          />
        </div>
      </div>
    </div>
  );
};

export default memo(EventTicketTypes);