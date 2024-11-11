// tickets/pages/TicketsListPage.jsx
import React, { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { supabase } from '@/services/supabaseClient';
import Header from '@/pages/visitor/header';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { AlertCircle, Printer, Eye, PrinterIcon } from 'lucide-react';
import TicketFilters from './ticket-filters';
import PrintableTicket from './printable-tickets';
import EventInformation from './event-infomation';

export default function TicketsListPage() {
  const [tickets, setTickets] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [isPrinting, setIsPrinting] = useState(false);
  const [selectedEvent, setSelectedEvent] = useState('all');
  const [selectedTicketType, setSelectedTicketType] = useState('all');
  const [events, setEvents] = useState([]);
  const [ticketTypes, setTicketTypes] = useState([]);

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

        const uniqueEvents = [...new Set(data.map(ticket => ticket.ticket_type.event.id))];
        const uniqueTicketTypes = [...new Set(data.map(ticket => ticket.ticket_type.id))];

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

  useEffect(() => {
    const style = document.createElement('style');
    style.innerHTML = `
      @media print {
        @page {
          size: 8.5in 2.75in;
          margin: 0;
        }
        body * {
          visibility: hidden;
        }
        .printable-ticket, .printable-ticket * {
          visibility: visible;
        }
        .printable-ticket {
          position: relative;
          page-break-after: always;
          width: 8.5in;
          height: 2.75in;
          background-color: white !important;
          color: black !important;
        }
        .printable-ticket * {
          color: black !important;
        }
        .printable-ticket svg {
          background-color: white !important;
        }
        .cut-line {
          border: none !important;
          border-top: 1px dashed #000 !important;
        }
        .scissors-icon {
          display: block !important;
        }
      }
    `;
    document.head.appendChild(style);
    return () => document.head.removeChild(style);
  }, []);

  const handlePrint = (ticketId) => {
    if (isPrinting) return;
    setIsPrinting(true);
    
    const ticketElement = document.getElementById(`printable-ticket-${ticketId}`);
    
    if (ticketElement) {
      const printWindow = window.open('', '_blank', 'width=800,height=600');
      printWindow.document.write(`
        <!DOCTYPE html>
        <html>
          <head>
            <title>Print Ticket</title>
            <style>
              @media print {
                @page {
                  size: 8.5in 2.75in landscape;
                  margin: 0;
                }
                
                html, body {
                  margin: 0;
                  padding: 0;
                  width: 8.5in;
                  height: 2.75in;
                  background: white;
                }
                
                .printable-ticket {
                  width: 8.5in !important;
                  height: 2.75in !important;
                  padding: 20px !important;
                  margin: 0 !important;
                  box-sizing: border-box !important;
                  background: white !important;
                  display: flex !important;
                  page-break-after: always !important;
                  position: relative !important;
                }
  
                .printable-ticket * {
                  visibility: visible !important;
                  color: black !important;
                  background-color: white !important;
                }
  
                .printable-ticket > div {
                  display: flex !important;
                }
  
                .printable-ticket > div > div:first-child {
                  flex: 3 !important;
                  border-right: 2px dashed #000 !important;
                  padding-right: 20px !important;
                }
  
                .printable-ticket > div > div:last-child {
                  flex: 1 !important;
                  padding-left: 20px !important;
                  align-items: center !important;
                  justify-content: center !important;
                }
  
                body > *:not(.printable-ticket) {
                  display: none !important;
                }
              }
            </style>
          </head>
          <body>
            ${ticketElement.outerHTML}
          </body>
        </html>
      `);
      
      printWindow.document.close();
    
      setTimeout(() => {
        printWindow.focus();
        printWindow.print();
        setTimeout(() => {
          printWindow.close();
          setIsPrinting(false);
        }, 500);
      }, 500);
    } else {
      setIsPrinting(false);
    }
  };

  const handlePrintAll = () => {
    if (isPrinting) return;
    setIsPrinting(true);

    const filteredTickets = getFilteredTickets();
    const printWindow = window.open('', '', 'width=800,height=600');
    const allTickets = filteredTickets.map(ticket => 
      document.getElementById(`printable-ticket-${ticket.id}`).outerHTML
    ).join('');

    printWindow.document.write(`
      <html>
        <head>
          <title>Print All Tickets</title>
          <style>
            @page {
              size: 8.5in 2.75in;
              margin: 0;
            }
            
            body {
              margin: 0;
              padding: 0;
            }
            
            .printable-ticket {
              width: 8.5in;
              height: 2.75in;
              padding: 0;
              box-sizing: border-box;
              page-break-after: always;
              position: relative;
            }
            
            .cut-line {
              border-top: 1px dashed #000;
            }
            
            .scissors-icon {
              position: absolute;
              width: 16px;
              height: 16px;
            }
            
            @media print {
              .cut-line {
                border-top: 1px dashed #000 !important;
              }
            }
          </style>
        </head>
        <body>
          ${allTickets}
        </body>
      </html>
    `);
    
    printWindow.document.close();
    printWindow.focus();
    
    setTimeout(() => {
      printWindow.print();
      setTimeout(() => {
        printWindow.close();
        setIsPrinting(false);
      }, 500);
    }, 250);
  };

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
        <Card className="max-w-2xl mx-auto mt-8">
          <CardContent className="pt-6">
            <div className="flex flex-col items-center space-y-4 text-center">
              <AlertCircle className="h-12 w-12 text-red-500" />
              <div className="space-y-2">
                <h2 className="text-2xl font-semibold dark:text-white">Error Loading Tickets</h2>
                <p className="text-gray-500 dark:text-gray-400">{error || 'No tickets found'}</p>
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
      <main className="max-w-6xl mx-auto p-4 pt-20">
        <div className="mb-6">
          <div className="flex justify-between items-center mb-4">
            <h1 className="text-3xl font-bold dark:text-white">My Tickets</h1>
            {filteredTickets.length > 0 && (
              <Button
                onClick={handlePrintAll}
                variant="outline"
                size="sm"
                className="print:hidden"
                disabled={isPrinting}
              >
                <PrinterIcon className="h-4 w-4 mr-2" />
                {isPrinting ? 'Printing...' : 'Print All Tickets'}
              </Button>
            )}
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
            <h2 className="text-2xl font-semibold mb-4 dark:text-white">{event.title}</h2>
            
            {Object.values(ticketTypes).map(({ type, tickets: typeTickets }) => (
              <div key={type.id} className="mb-6">
                <h3 className="text-xl font-medium mb-3 dark:text-white pl-4 border-l-4 border-primary">
                  {type.name} ({typeTickets.length})
                </h3>
                <div className="grid gap-6 md:grid-cols-1">
                  {typeTickets.map((ticket) => (
                    <Card key={ticket.id} className="relative">
                      <CardContent className="p-6">
                        <PrintableTicket 
                          ticket={ticket}
                          event={event}
                          type={type}
                        />

                        <div className="mt-4 flex gap-2 print:hidden">
                          <Button
                            onClick={() => handlePrint(ticket.id)}
                            variant="outline"
                            size="sm"
                            disabled={isPrinting}
                          >
                            <Printer className="h-4 w-4 mr-2" />
                            {isPrinting ? 'Printing...' : 'Print Ticket'}
                          </Button>
                          <Link to={`/tickets/${ticket.id}`}>
                            <Button variant="outline" size="sm">
                              <Eye className="h-4 w-4 mr-2" />
                              View Details
                            </Button>
                          </Link>
                        </div>
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
          <Card className="mt-4">
            <CardContent className="pt-6">
              <div className="flex flex-col items-center space-y-4 text-center">
                <AlertCircle className="h-12 w-12 text-yellow-500" />
                <div className="space-y-2">
                  <h2 className="text-xl font-semibold dark:text-white">No Tickets Found</h2>
                  <p className="text-gray-500 dark:text-gray-400">
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