import React from 'react';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export default function TicketFilters({ 
  selectedEvent, 
  setSelectedEvent, 
  selectedTicketType, 
  setSelectedTicketType,
  events,
  ticketTypes 
}) {
  return (
    <div className="flex gap-4 mb-6">
      <div className="flex-1">
        <Select
          value={selectedEvent}
          onValueChange={setSelectedEvent}
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder="Filter by Event" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Events</SelectItem>
            {events.map(event => (
              <SelectItem key={event.id} value={event.id}>
                {event.title}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="flex-1">
        <Select
          value={selectedTicketType}
          onValueChange={setSelectedTicketType}
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder="Filter by Ticket Type" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Ticket Types</SelectItem>
            {ticketTypes.map(type => (
              <SelectItem key={type.id} value={type.id}>
                {type.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}