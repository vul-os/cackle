import React, { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
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
} from 'lucide-react';

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

export default function TicketPage() {
  const { id } = useParams();
  const [ticket, setTicket] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    const fetchTicket = async () => {
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
          `)
          .eq('id', id)
          .single();

        if (fetchError) throw fetchError;
        if (!data) throw new Error('Ticket not found');

        setTicket(data);
      } catch (err) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    };

    if (id) {
      fetchTicket();
    }
  }, [id]);

  // Add print-specific styles
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
        #printable-ticket, #printable-ticket * {
          visibility: visible;
        }
        #printable-ticket {
          position: absolute;
          left: 0;
          top: 0;
          width: 8.5in;
          height: 2.75in;
        }
      }
    `;
    document.head.appendChild(style);
    return () => document.head.removeChild(style);
  }, []);

  if (loading) {
    return (
      <>
        <Header />
        <div className="flex items-center justify-center min-h-[400px]">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900" />
        </div>
      </>
    );
  }

  if (error || !ticket) {
    return (
      <>
        <Header />
        <Card className="max-w-2xl mx-auto mt-8">
          <CardContent className="pt-6">
            <div className="flex flex-col items-center space-y-4 text-center">
              <AlertCircle className="h-12 w-12 text-red-500" />
              <div className="space-y-2">
                <h2 className="text-2xl font-semibold">Error Loading Ticket</h2>
                <p className="text-gray-500">{error || 'Ticket not found'}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </>
    );
  }

  const event = ticket.ticket_type.event;
  const ticketType = ticket.ticket_type;

  const handlePrint = () => {
    window.print();
  };

  return (
    <>
      <Header />
      <main className="max-w-4xl mx-auto p-4 pt-20">
        <Card className="mb-6">
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle>Ticket Details</CardTitle>
            <Button 
              onClick={handlePrint}
              className="print:hidden"
              variant="outline"
            >
              <Printer className="h-4 w-4 mr-2" />
              Print Ticket
            </Button>
          </CardHeader>
          <CardContent>
            {/* Printable Ticket Section */}
            <div 
              id="printable-ticket" 
              className="bg-white p-6 rounded-lg border-2 border-dashed print:border-solid print:rounded-none print:p-4"
            >
              <div className="flex print:h-[2.75in]">
                {/* Main Ticket Content */}
                <div className="flex-[3] pr-6 print:pr-4 print:border-r-2 print:border-dashed">
                  <div className="space-y-4">
                    <h2 className="text-2xl font-bold">{event.title}</h2>
                    <div className="flex items-center text-gray-600">
                      <Tag className="h-4 w-4 mr-2" />
                      <span className="text-sm">{ticketType.name}</span>
                    </div>
                    <div className="flex items-center text-gray-600">
                      <Calendar className="h-4 w-4 mr-2" />
                      <span className="text-sm">{formatDate(event.start_time)}</span>
                    </div>
                    <div className="flex items-center text-gray-600">
                      <Clock className="h-4 w-4 mr-2" />
                      <span className="text-sm">{formatTime(event.start_time)}</span>
                    </div>
                    <div className="flex items-center text-gray-600">
                      <MapPin className="h-4 w-4 mr-2" />
                      <span className="text-sm">
                        {event.venue_name} - {event.venue_address}
                      </span>
                    </div>
                    <div className="text-xs font-mono mt-4">
                      Ticket ID: {ticket.ticket_code}
                    </div>
                  </div>
                </div>

                {/* QR Code Section */}
                <div className="flex-1 flex flex-col items-center justify-center">
                  <div className="print:p-0 p-4">
                    <QRCodeSVG 
                      value={`https://cackle.co.za/tickets/code/${ticket.ticket_code}`}
                      size={100}
                      level="H"
                      includeMargin={true}
                    />
                  </div>
                  <div className="text-sm font-mono mt-2 text-center">
                    {ticket.ticket_code}
                  </div>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Additional Event Information - Hidden during print */}
        <div className="print:hidden">
          {event.description && (
            <Card className="mb-6">
              <CardHeader>
                <CardTitle>Event Description</CardTitle>
              </CardHeader>
              <CardContent>
                <p className="text-gray-600 whitespace-pre-wrap">{event.description}</p>
              </CardContent>
            </Card>
          )}

          {event.information && (
            <Card className="mb-6">
              <CardHeader>
                <CardTitle>Event Information</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="prose max-w-none">
                  <div dangerouslySetInnerHTML={{ __html: event.information }} />
                </div>
              </CardContent>
            </Card>
          )}

          {event.policy_info && (
            <Card>
              <CardHeader>
                <CardTitle>Event Policies</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="prose max-w-none">
                  <div dangerouslySetInnerHTML={{ __html: event.policy_info }} />
                </div>
              </CardContent>
            </Card>
          )}
        </div>
      </main>
    </>
  );
}