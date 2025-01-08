// tickets/pages/TicketsListPage.jsx
import React, { useState, useEffect } from 'react';
import { supabase } from '@/services/supabaseClient';
import Header from '@/pages/visitor/header';
import { Card, CardContent } from '@/components/ui/card';
import { AlertCircle } from 'lucide-react';
import TicketFilters from './ticket-filters';
import PrintableTicket from './printing/layout';
import EventInformation from './event-infomation';
import { usePrintTicket } from './printing/use-print-ticket';
import { PrintTicketButtons, PrintAllButton } from './printing/print-buttons';

export default function TicketsListPage() {
  const [tickets, setTickets] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [selectedEvent, setSelectedEvent] = useState('all');
  const [selectedTicketType, setSelectedTicketType] = useState('all');
  const [events, setEvents] = useState([]);
  const [ticketTypes, setTicketTypes] = useState([]);

  const { isPrinting, printSingleTicket, printAllTickets } = usePrintTicket();

  useEffect(() => {
    const fetchTickets = async () => {
      try {
        setLoading(true);
        const { data, error: fetchError } = await supabase
          .from('tickets')
          .select(`
            *,
            ticket_type:ticket_types (
              *,
              event:events (*)
            )
          `);

        if (fetchError) throw fetchError;
        if (!data) throw new Error('No tickets found');

        setTickets(data);

        // Extract unique events and ticket types
        setEvents(data.reduce((acc, ticket) => {
          if (!acc.find(e => e.id === ticket.ticket_type.event.id)) {
            acc.push(ticket.ticket_type.event);
          }
          return acc;
        }, []));

        setTicketTypes(data.reduce((acc, ticket) => {
          if (!acc.find(t => t.id === ticket.ticket_type.id)) {
            acc.push(ticket.ticket_type);
          }
          return acc;
        }, []));

      } catch (err) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    };

    fetchTickets();
  }, []);

  const getFilteredTickets = () => {
    return tickets.filter(ticket => {
      const eventMatch = selectedEvent === 'all' || ticket.ticket_type.event.id === selectedEvent;
      const typeMatch = selectedTicketType === 'all' || ticket.ticket_type.id === selectedTicketType;
      return eventMatch && typeMatch;
    });
  };

  if (loading) {
    return (
      <>
        <Header />
        <div className="flex items-center justify-center min-h-[400px]">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900 dark:border-white" />
        </div>
      </>
    );
  }

  if (error || !tickets.length) {
    return (
      <>
        <Header />
        <Card className="max-w-2xl mx-auto mt-8 bg-white dark:bg-gray-800">
          <CardContent className="pt-6">
            <div className="flex flex-col items-center space-y-4 text-center">
              <AlertCircle className="h-12 w-12 text-red-500 dark:text-red-400" />
              <div className="space-y-2">
                <h2 className="text-2xl font-semibold text-gray-900 dark:text-white">Error Loading Tickets</h2>
                <p className="text-gray-700 dark:text-white">{error || 'No tickets found'}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </>
    );
  }

  const filteredTickets = getFilteredTickets();

  const groupedTickets = filteredTickets.reduce((acc, ticket) => {
    const eventId = ticket.ticket_type.event.id;
    const ticketTypeId = ticket.ticket_type.id;
    
    if (!acc[eventId]) {
      acc[eventId] = {
        event: ticket.ticket_type.event,
        ticketTypes: {}
      };
    }
    
    if (!acc[eventId].ticketTypes[ticketTypeId]) {
      acc[eventId].ticketTypes[ticketTypeId] = {
        type: ticket.ticket_type,
        tickets: []
      };
    }
    
    acc[eventId].ticketTypes[ticketTypeId].tickets.push(ticket);
    return acc;
  }, {});

  return (
    <>
      <Header />
      <main className="max-w-6xl mx-auto p-4 pt-20 bg-white dark:bg-gray-900">
        <div className="mb-6">
          <div className="flex justify-between items-center mb-4">
            <h1 className="text-3xl font-bold text-gray-900 dark:text-white">My Tickets</h1>
            <PrintAllButton 
              onPrintAll={() => printAllTickets(filteredTickets)}
              isPrinting={isPrinting}
              ticketsCount={filteredTickets.length}
            />
          </div>

          <TicketFilters
            selectedEvent={selectedEvent}
            setSelectedEvent={setSelectedEvent}
            selectedTicketType={selectedTicketType}
            setSelectedTicketType={setSelectedTicketType}
            events={events}
            ticketTypes={ticketTypes}
          />
        </div>
        
        {Object.values(groupedTickets).map(({ event, ticketTypes }) => (
          <div key={event.id} className="mb-8">
            <h2 className="text-2xl font-semibold mb-4 text-gray-900 dark:text-white">
              {event.title}
            </h2>
            
            {Object.values(ticketTypes).map(({ type, tickets: typeTickets }) => (
              <div key={type.id} className="mb-6">
                <h3 className="text-xl font-medium mb-3 text-gray-800 dark:text-white pl-4 border-l-4 border-primary">
                  {type.name} ({typeTickets.length})
                </h3>
                <div className="grid gap-6 md:grid-cols-1">
                  {typeTickets.map((ticket) => (
                    <Card key={ticket.id} className="relative bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 shadow-sm">
                      <CardContent className="p-6">
                        <PrintableTicket 
                          ticket={ticket}
                          event={event}
                          type={type}
                        />

                        <PrintTicketButtons 
                          ticketId={ticket.id}
                          onPrint={() => printSingleTicket(ticket.id)}
                          isPrinting={isPrinting}
                        />
                      </CardContent>
                    </Card>
                  ))}
                </div>
              </div>
            ))}

            <EventInformation event={event} />
          </div>
        ))}

        {filteredTickets.length === 0 && (
          <Card className="mt-4 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700">
            <CardContent className="pt-6">
              <div className="flex flex-col items-center space-y-4 text-center">
                <AlertCircle className="h-12 w-12 text-yellow-500 dark:text-yellow-400" />
                <div className="space-y-2">
                  <h2 className="text-xl font-semibold text-gray-900 dark:text-white">No Tickets Found</h2>
                  <p className="text-gray-700 dark:text-white">
                    No tickets match your current filter settings. Try adjusting your filters.
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>
        )}
      </main>
    </>
  );
}