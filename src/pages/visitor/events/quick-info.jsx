import React from 'react';
import { Calendar, MapPin } from 'lucide-react';
import TicketSelection from './ticket-selection';

const QuickInfoItem = ({ icon, title, subtitle }) => (
  <div className="flex items-center gap-4 group cursor-pointer transform hover:scale-105 transition-transform duration-300">
    <div className="p-3 rounded-full bg-gradient-to-br from-[#880424]/10 to-[#660318]/10 dark:from-[#880424]/5 dark:to-[#660318]/5 group-hover:from-[#880424]/20 group-hover:to-[#660318]/20 transition-colors">
      {icon}
    </div>
    <div>
      <p className="font-semibold text-gray-900 dark:text-gray-100">
        {title}
      </p>
      <p className="text-gray-600 dark:text-gray-300">
        {subtitle}
      </p>
    </div>
  </div>
);

const EventQuickInfo = ({ event, ticketTypes }) => {
  const formatDate = (date) => {
    return date ? new Date(date).toLocaleDateString('en-US', {
      month: 'long',
      day: 'numeric',
      year: 'numeric'
    }) : 'Date TBA';
  };

  const formatTime = (startTime, endTime) => {
    const formatTimeString = (timestamp) => {
      if (!timestamp) return '';
      return new Date(timestamp).toLocaleTimeString('en-US', {
        hour: 'numeric',
        minute: '2-digit',
        hour12: true
      });
    };

    return `${formatTimeString(startTime)} - ${formatTimeString(endTime)}`;
  };

  return (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
      <QuickInfoItem 
        icon={<Calendar className="h-6 w-6 text-[#880424] dark:text-[#990525]" />}
        title={formatDate(event.start_time)}
        subtitle={formatTime(event.start_time, event.end_time)}
      />
      
      <QuickInfoItem 
        icon={<MapPin className="h-6 w-6 text-[#880424] dark:text-[#990525]" />}
        title={event.venue_name || 'Venue'}
        subtitle={
          <span className="text-[#880424] dark:text-[#990525] group-hover:underline">
            {event.venue_address || 'View on map'}
          </span>
        }
      />
      
      <div className="flex items-center justify-end">
        <TicketSelection
          ticketTypes={ticketTypes}
          event={event}
        />
      </div>
    </div>
  );
};

export default EventQuickInfo;