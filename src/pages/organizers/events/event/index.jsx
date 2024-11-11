import React, { useEffect, useState, useContext } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { supabase } from '@/services/supabaseClient';
import { AuthContext } from '@/context/use-auth';
import { useToast } from "@/components/ui/use-toast";
import { Button } from '@/components/ui/button';
import { EventPageHeader } from './header';
import { EventDetailsCard } from './details';
import { DeleteEventDialog } from './delete-dialog';
import { useEventForm } from './event-form-hook';
import { Spinner } from '@/components/ui/spinner';

const EventPage = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const { activeOrganization } = useContext(AuthContext);
  const { toast } = useToast();
  const [event, setEvent] = useState(null);
  const [loading, setLoading] = useState(true);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  
  const {
    editForm,
    dateRange,
    setDateRange,
    hasChanges,
    handleInputChange,
    categories,
    subcategories,
    availableSubcategories,
    initializeForm
  } = useEventForm();

  useEffect(() => {
    if (id) {
      fetchEvent();
    }
  }, [id]);

  // Update dateRange whenever editForm's dates change
  useEffect(() => {
    if (editForm.start_time && editForm.end_time) {
      setDateRange({
        from: new Date(editForm.start_time),
        to: new Date(editForm.end_time)
      });
    }
  }, [editForm.start_time, editForm.end_time]);

  const fetchEvent = async () => {
    try {
      setLoading(true);
      const { data, error } = await supabase
        .from('events')
        .select('*')
        .eq('id', id)
        .single();

      if (error) throw error;
      
      // Initialize both event and form data
      setEvent(data);
      initializeForm({
        ...data,
        // Ensure dates are properly formatted
        start_time: data.start_time ? new Date(data.start_time).toISOString() : null,
        end_time: data.end_time ? new Date(data.end_time).toISOString() : null,
      });
      
    } catch (error) {
      console.error('Error fetching event:', error);
      toast({
        title: "Error",
        description: "Failed to fetch event details",
        variant: "destructive"
      });
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async () => {
    try {
      setIsSubmitting(true);
      const { error } = await supabase
        .from('events')
        .delete()
        .eq('id', id);

      if (error) throw error;

      toast({
        title: "Success",
        description: "Event deleted successfully"
      });
      navigate('/admin/events');
    } catch (error) {
      console.error('Error deleting event:', error);
      toast({
        title: "Error",
        description: "Failed to delete event",
        variant: "destructive"
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleSave = async () => {
    try {
      setIsSubmitting(true);
      
      // Ensure dates are in the correct format
      const updatedForm = {
        ...editForm,
        start_time: editForm.start_time ? new Date(editForm.start_time).toISOString() : null,
        end_time: editForm.end_time ? new Date(editForm.end_time).toISOString() : null,
      };

      const { error } = await supabase
        .from('events')
        .update(updatedForm)
        .eq('id', id);

      if (error) throw error;

      toast({
        title: "Success",
        description: "Event updated successfully"
      });
      
      // Refresh event data
      await fetchEvent();
      
    } catch (error) {
      console.error('Error updating event:', error);
      toast({
        title: "Error",
        description: "Failed to update event",
        variant: "destructive"
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleDateChange = (newDateRange) => {
    if (newDateRange?.from) {
      handleInputChange('start_time', newDateRange.from.toISOString());
    }
    if (newDateRange?.to) {
      handleInputChange('end_time', newDateRange.to.toISOString());
    }
    setDateRange(newDateRange);
  };

  if (loading) return <Spinner />;
  if (!event) return <div>Event not found</div>;

  return (
    <div className="min-h-screen bg-white p-4 md:p-8">
      <div className="max-w-4xl mx-auto">
        <EventPageHeader
          editForm={editForm}
          handleInputChange={handleInputChange}
          handleSave={handleSave}
          hasChanges={hasChanges}
          setShowDeleteDialog={setShowDeleteDialog}
          navigate={navigate}
          isSubmitting={isSubmitting}
        />
        
        <EventDetailsCard
          editForm={editForm}
          handleInputChange={handleInputChange}
          initialData={event}
          isSubmitting={isSubmitting}
          organizationId={activeOrganization?.id}
          categories={categories}
          availableSubcategories={availableSubcategories}
          dateRange={dateRange}
          onDateChange={handleDateChange}
        />
      </div>

      <DeleteEventDialog
        open={showDeleteDialog}
        onOpenChange={setShowDeleteDialog}
        onDelete={handleDelete}
      />
    </div>
  );
};

export default EventPage;