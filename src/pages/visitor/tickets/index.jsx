import React, { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { supabase } from '@/services/supabaseClient';
import { QRCodeSVG } from 'qrcode.react';
import { format } from 'date-fns';
import Header from '@/pages/visitor/header';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import {
  Calendar,
  Clock,
  MapPin,
  Tag,
  AlertCircle,
  Printer,
  Eye,
  PrinterIcon,
  Filter,
} from 'lucide-react';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

function formatDate(dateString) {
  if (!dateString) return 'N/A';
  try {
    return format(new Date(dateString), 'EEEE, MMMM d, yyyy');
  } catch (error) {
    return 'N/A';
  }
}

function formatTime(dateString) {
  if (!dateString) return 'N/A';
  try {
    return format(new Date(dateString), 'h:mm a');
  } catch (error) {
    return 'N/A';
  }
}

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

        // Extract unique events and ticket types
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
      const printWindow = window.open('', '', 'width=800,height=600');
      printWindow.document.write(`
        <html>
          <head>
            <title>Print Ticket</title>
            <style>
              @page {
                size: A4;
                margin: 20mm;
              }
              
              body {
                margin: 0;
                padding: 0;
                width: 210mm;
                height: 297mm;
              }
              
              .printable-ticket {
                width: 100%;
                height: auto;
                padding: 10mm;
                box-sizing: border-box;
              }
              
              @media print {
                html, body {
                  width: 210mm;
                  height: 297mm;
                }
                
                .printable-ticket {
                  page-break-inside: avoid;
                  break-inside: avoid;
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
      printWindow.focus();
      
      setTimeout(() => {
        printWindow.print();
        setTimeout(() => {
          printWindow.close();
          setIsPrinting(false);
        }, 500);
      }, 250);
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
              size: A4;
              margin: 20mm;
            }
            
            body {
              margin: 0;
              padding: 0;
              width: 210mm;
              height: 297mm;
            }
            
            .printable-ticket {
              width: 100%;
              height: auto;
              padding: 10mm;
              box-sizing: border-box;
              page-break-after: always;
            }
            
            @media print {
              html, body {
                width: 210mm;
                height: 297mm;
              }
              
              .printable-ticket {
                page-break-inside: avoid;
                break-inside: avoid;
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

  // Group tickets by event and then by ticket type
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
                        <div 
                          id={`printable-ticket-${ticket.id}`}
                          className="printable-ticket bg-white dark:bg-gray-800 rounded-lg border-2 border-dashed print:border-solid print:rounded-none dark:border-gray-600"
                        >
                          <div className="flex print:h-[2.75in]">
                            <div className="flex-[3] pr-6 print:pr-4 print:border-r-2 print:border-dashed dark:print:border-gray-600">
                              <div className="space-y-4">
                                <h2 className="text-2xl font-bold dark:text-white">{event.title}</h2>
                                <div className="flex items-center text-gray-600 dark:text-gray-300">
                                  <Tag className="h-4 w-4 mr-2" />
                                  <span className="text-sm">{type.name}</span>
                                </div>
                                <div className="flex items-center text-gray-600 dark:text-gray-300">
                                  <Calendar className="h-4 w-4 mr-2" />
                                  <span className="text-sm">{formatDate(event.start_time)}</span>
                                </div>
                                <div className="flex items-center text-gray-600 dark:text-gray-300">
                                  <Clock className="h-4 w-4 mr-2" />
                                  <span className="text-sm">{formatTime(event.start_time)}</span>
                                </div>
                                <div className="flex items-center text-gray-600 dark:text-gray-300">
                                  <MapPin className="h-4 w-4 mr-2" />
                                  <span className="text-sm">
                                    {event.venue_name} - {event.venue_address}
                                  </span>
                                </div>
                                <div className="text-xs font-mono mt-4 dark:text-gray-300">
                                  Ticket ID: {ticket.ticket_code}
                                </div>
                              </div>
                            </div>

                            <div className="flex-1 flex flex-col items-center justify-center">
                              <div className="print:p-0 p-4"><QRCodeSVG 
                                  value={`https://cackle.co.za/tickets/code/${ticket.ticket_code}`}
                                  size={100}
                                  level="H"
                                  includeMargin={true}
                                  className="dark:bg-white p-2 rounded"
                                />
                              </div>
                              <div className="text-sm font-mono mt-2 text-center dark:text-gray-300">
                                {ticket.ticket_code}
                              </div>
                            </div>
                          </div>
                        </div>

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

            {/* Event Information Section */}
            {(event.description || event.information || event.policy_info) && (
              <Card className="mt-4 print:hidden">
                <CardHeader>
                  <CardTitle className="dark:text-white">{event.title} - Event Information</CardTitle>
                </CardHeader>
                <CardContent className="space-y-6">
                  {event.description && (
                    <div>
                      <h3 className="font-semibold mb-2 dark:text-white">Description</h3>
                      <p className="text-gray-600 dark:text-gray-300 whitespace-pre-wrap">{event.description}</p>
                    </div>
                  )}

                  {event.information && (
                    <div>
                      <h3 className="font-semibold mb-2 dark:text-white">Additional Information</h3>
                      <div className="prose dark:prose-invert max-w-none">
                        <div dangerouslySetInnerHTML={{ __html: event.information }} />
                      </div>
                    </div>
                  )}

                  {event.policy_info && (
                    <div>
                      <h3 className="font-semibold mb-2 dark:text-white">Event Policies</h3>
                      <div className="prose dark:prose-invert max-w-none">
                        <div dangerouslySetInnerHTML={{ __html: event.policy_info }} />
                      </div>
                    </div>
                  )}
                </CardContent>
              </Card>
            )}
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