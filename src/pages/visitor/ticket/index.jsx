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
  id={`printable-ticket-${ticket.id}`}
  className="printable-ticket"
>
  <div className="flex p-6">
    {/* Left section */}
    <div className="flex-[3] pr-8 border-r border-dashed border-gray-300">
      <div className="flex justify-between items-start mb-6">
        <div>
          <h2 className="text-2xl font-bold mb-1">
            {event.title}
          </h2>
          <p className="text-sm font-medium">
            {type.name}
          </p>
        </div>
        <div className="text-sm font-mono">
          #{ticket.ticket_code}
        </div>
      </div>

      {/* Event details */}
      <div className="space-y-4 mb-6">
        <div className="flex items-center space-x-3">
          <Calendar className="h-5 w-5" />
          <div>
            <div className="text-sm font-medium">
              {formatDate(event.start_time)}
            </div>
            <div className="text-sm">
              {formatTime(event.start_time)}
            </div>
          </div>
        </div>

        <div className="flex items-start space-x-3">
          <MapPin className="h-5 w-5 mt-0.5" />
          <div>
            <div className="text-sm font-medium">
              {event.venue_name}
            </div>
            <div className="text-sm">
              {event.venue_address}
            </div>
          </div>
        </div>
      </div>

      <div className="pt-4 border-t border-dashed">
        <p className="text-xs">
          Please present this ticket at the entrance. Valid for one-time entry only.
        </p>
      </div>
    </div>

    {/* Right section - QR Code */}
    <div className="flex-1 pl-8 flex flex-col items-center justify-center">
      <div className="bg-white p-3 rounded-xl">
      <QRCodeSVG 
  value={`https://cackle.co.za/tickets/code/${ticket.ticket_code}`}
  size={120}s
  includeMargin={false}
  className="qr-code"
/>
      </div>
      <div className="mt-4 text-sm font-mono text-center">
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