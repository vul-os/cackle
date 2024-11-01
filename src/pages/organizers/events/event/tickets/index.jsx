"use client";

import React, { memo } from 'react';
import { useParams, useNavigate, Outlet } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { ArrowLeft } from 'lucide-react';
import {
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from "@/components/ui/tabs";

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

const EventTicketsLayout = () => {
  const { id: eventId } = useParams();
  const navigate = useNavigate();

  const handleTabChange = (value) => {
    navigate(`/admin/events/${eventId}/tickets${value === 'types' ? '/types' : ''}`);
  };

  return (
    <div className="min-h-screen bg-white p-4 md:p-8">
      <div className="max-w-6xl mx-auto">
        <BackButton eventId={eventId} navigate={navigate} />
        
        <Tabs 
          defaultValue="tickets" 
          className="w-full"
          onValueChange={handleTabChange}
        >
          <TabsList className="mb-6">
            <TabsTrigger value="tickets">Tickets</TabsTrigger>
            <TabsTrigger value="types">Ticket Types</TabsTrigger>
          </TabsList>
          <Outlet />
        </Tabs>
      </div>
    </div>
  );
};

export default memo(EventTicketsLayout);