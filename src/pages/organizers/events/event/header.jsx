import React from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ArrowLeft, Save, Trash2, Ticket, DollarSign, Users } from 'lucide-react';

export const EventPageHeader = ({
  editForm,
  handleInputChange,
  handleSave,
  hasChanges,
  setShowDeleteDialog,
  navigate
}) => {
  const handleAttendeeClick = () => {
    console.log('Current event ID:', editForm.id); // Debug log
    console.log('Navigation path:', `/admin/events/${editForm.id}/attendees`); // Debug log
    navigate(`/admin/events/${editForm.id}/attendees`);
  };

  return (
    <div className="mb-8">
      <Button
        variant="ghost"
        onClick={() => navigate('/admin/events')}
        className="mb-4 hover:bg-gray-100"
      >
        <ArrowLeft className="h-4 w-4 mr-2" />
        Back to Events
      </Button>

      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
        <div className="flex-1">
          <Input
            value={editForm.title}
            onChange={(e) => handleInputChange('title', e.target.value)}
            className="text-2xl md:text-3xl font-bold text-gray-900 border-transparent hover:border-gray-200 transition-colors bg-transparent p-2 h-auto focus-visible:ring-1"
            placeholder="Event Title"
          />
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            onClick={handleAttendeeClick}
            className="bg-white hover:bg-gray-50 transition-colors"
          >
            <Users className="h-4 w-4 mr-2" />
            Orders
          </Button>
          <Button
            variant="outline"
            onClick={() => navigate(`/admin/events/${editForm.id}/tickets`)}
            className="bg-white hover:bg-gray-50 transition-colors"
          >
            <Ticket className="h-4 w-4 mr-2" />
            Manage Tickets
          </Button>
          <Button
            variant="outline"
            onClick={() => navigate(`/admin/events/${editForm.id}/payouts`)}
            className="bg-white hover:bg-gray-50 transition-colors"
          >
            <DollarSign className="h-4 w-4 mr-2" />
            Payouts
          </Button>
        </div>
      </div>
    </div>
  );
};