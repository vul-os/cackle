import React from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { Calendar, MapPin, Tag, Scissors } from 'lucide-react';
import { formatDate, formatTime } from '../date-utils';

export default function TicketLayout({ ticket, event, type }) {
  return (
    <div className="p-8 print:p-0 print:m-0">
      <div 
        id={`printable-ticket-${ticket.id}`}
        className="relative bg-gradient-to-r from-gray-50 to-white dark:from-gray-800 dark:to-gray-900 rounded-lg overflow-visible shadow-lg border border-dashed border-gray-300 dark:border-gray-700 print:shadow-none print:border-gray-300 print:dark:border-gray-300 print:bg-white print:from-white print:to-white"
      >
        {/* Scissors Icons - hidden in print */}
        <div className="absolute -top-4 -left-4 print:hidden">
          <Scissors className="h-6 w-6 text-gray-400 transform -rotate-45" />
        </div>
        <div className="absolute -top-4 -right-4 print:hidden">
          <Scissors className="h-6 w-6 text-gray-400 transform rotate-45" />
        </div>
        <div className="absolute -bottom-4 -left-4 print:hidden">
          <Scissors className="h-6 w-6 text-gray-400 transform -rotate-135" />
        </div>
        <div className="absolute -bottom-4 -right-4 print:hidden">
          <Scissors className="h-6 w-6 text-gray-400 transform rotate-135" />
        </div>

        {/* Main Ticket Content */}
        <div className="flex p-6 print:p-4 print:min-h-[2.75in]">
          <div className="flex-[3] pr-8 border-r border-dashed border-gray-300 print:border-gray-400">
            <div className="flex justify-between items-start mb-6">
              <div>
                <h2 className="text-3xl font-bold text-gray-900 print:text-black mb-1">
                  {event.title}
                </h2>
                <p className="text-sm font-medium text-primary print:text-gray-700">
                  {type.name}
                </p>
              </div>
              <div className="text-sm font-mono text-gray-500 print:text-gray-600">
                #{ticket.ticket_code}
              </div>
            </div>

            <div className="grid grid-cols-2 gap-8 mb-6 print:gap-4">
              <div className="space-y-4">
                <div className="flex items-center space-x-3">
                  <Calendar className="h-5 w-5 text-primary print:text-gray-700" />
                  <div>
                    <div className="text-sm font-medium text-gray-900 print:text-black">
                      {formatDate(event.start_time)}
                    </div>
                    <div className="text-sm text-gray-500 print:text-gray-600">
                      {formatTime(event.start_time)}
                    </div>
                  </div>
                </div>

                <div className="flex items-start space-x-3">
                  <MapPin className="h-5 w-5 text-primary print:text-gray-700 mt-0.5" />
                  <div>
                    <div className="text-sm font-medium text-gray-900 print:text-black">
                      {event.venue_name}
                    </div>
                    <div className="text-sm text-gray-500 print:text-gray-600">
                      {event.venue_address}
                    </div>
                  </div>
                </div>
              </div>

              <div className="space-y-4">
                <div className="flex items-center space-x-3">
                  <Tag className="h-5 w-5 text-primary print:text-gray-700" />
                  <div className="text-sm font-medium text-gray-900 print:text-black">
                    {type.name}
                  </div>
                </div>
              </div>
            </div>

            <div className="pt-4 border-t border-dashed border-gray-200 print:border-gray-300">
              <p className="text-xs text-gray-500 print:text-gray-600">
                Please present this ticket at the entrance. Valid for one-time entry only.
              </p>
            </div>
          </div>

          <div className="flex-1 pl-8 flex flex-col items-center justify-center print:pl-4">
            <div className="bg-white p-3 rounded-xl shadow-sm print:shadow-none print:p-2">
              <QRCodeSVG 
                value={`https://cackle.co.za/tickets/code/${ticket.ticket_code}`}
                size={120}
                level="H"
                includeMargin={false}
              />
            </div>
            <div className="mt-4 text-sm font-mono text-center text-gray-600 print:text-gray-700">
              {ticket.ticket_code}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}