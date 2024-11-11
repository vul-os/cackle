import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { supabase } from '@/services/supabaseClient';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Alert, AlertDescription } from '@/components/ui/alert';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { 
  ArrowLeft, 
  AlertTriangle, 
  DollarSign,
  Calendar,
  Clock,
  Loader2,
  CheckCircle2,
  XCircle,
  PiggyBank
} from 'lucide-react';
import { toast } from '@/components/ui/use-toast';

const PayoutsPage = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [event, setEvent] = useState(null);
  const [payout, setPayout] = useState(null);
  const [loading, setLoading] = useState(true);
  const [processing, setProcessing] = useState(false);
  const [showEndEventDialog, setShowEndEventDialog] = useState(false);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const { data: eventData, error: eventError } = await supabase
          .from('events')
          .select('*')
          .eq('id', id)
          .single();

        if (eventError) throw eventError;

        const { data: payoutData, error: payoutError } = await supabase
          .from('event_payouts')
          .select('*')
          .eq('event_id', id)
          .single();

        if (payoutError && payoutError.code !== 'PGRST116') {
          throw payoutError;
        }

        setEvent(eventData);
        setPayout(payoutData || null);
      } catch (err) {
        toast({
          title: "Error",
          description: "Failed to load event details.",
          variant: "destructive"
        });
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, [id]);

  const handleEndEvent = async () => {
    try {
      setProcessing(true);

      // Update event completed_at
      const { error: updateError } = await supabase
        .from('events')
        .update({ completed_at: new Date().toISOString() })
        .eq('id', id);

      if (updateError) throw updateError;

      // Calculate total revenue
      const { data: ticketData, error: ticketError } = await supabase
        .from('tickets')
        .select('ticket_types(price)')
        .eq('event_id', id)
        .eq('status', 'valid');

      if (ticketError) throw ticketError;

      const totalRevenue = ticketData.reduce((sum, ticket) => 
        sum + (ticket.ticket_types?.price || 0), 0);

      // Create payout record
      const { error: payoutError } = await supabase
        .from('event_payouts')
        .insert({
          event_id: id,
          organization_id: event.organization_id,
          amount: totalRevenue,
          currency: 'ZAR',
          status: 'pending'
        });

      if (payoutError) throw payoutError;

      toast({
        title: "Success!",
        description: "Event has been ended and payout process initiated.",
      });
      
      window.location.reload();
    } catch (err) {
      toast({
        title: "Error",
        description: "Failed to end event. Please try again.",
        variant: "destructive"
      });
    } finally {
      setProcessing(false);
      setShowEndEventDialog(false);
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gradient-to-b from-red-50 to-white p-8 flex items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-red-600" />
      </div>
    );
  }

  const isEventEnded = event?.completed_at !== null;
  
  return (
    <div className="min-h-screen bg-gradient-to-b from-red-50 to-white p-8">
      <div className="max-w-4xl mx-auto">
        <Button
          variant="ghost"
          onClick={() => navigate(-1)}
          className="mb-8 hover:bg-white/50"
        >
          <ArrowLeft className="h-4 w-4 mr-2" />
          Back to Event
        </Button>

        <div className="text-center mb-12">
          <h1 className="text-4xl font-bold text-gray-900 mb-4">{event.title}</h1>
          <p className="text-xl text-gray-600">Manage Your Event Payouts</p>
        </div>

        {!isEventEnded && (
          <Alert className="mb-8 bg-amber-50 border-amber-200">
            <AlertTriangle className="h-5 w-5 text-amber-600" />
            <AlertDescription className="text-amber-800">
              Your event is still active. End the event to process payouts and view final settlements.
            </AlertDescription>
          </Alert>
        )}

        <Card className="mb-8 shadow-sm hover:shadow-md transition-shadow">
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="text-2xl">Event Status</CardTitle>
                <CardDescription>Current status and financial overview</CardDescription>
              </div>
              {!isEventEnded && (
                <AlertDialog open={showEndEventDialog} onOpenChange={setShowEndEventDialog}>
                  <AlertDialogTrigger asChild>
                    <Button variant="destructive" className="bg-red-600 hover:bg-red-700">
                      End Event
                    </Button>
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>Are you absolutely sure?</AlertDialogTitle>
                      <AlertDialogDescription className="space-y-3">
                        <p>This action cannot be undone. Ending the event will:</p>
                        <ul className="list-disc ml-6 space-y-2">
                          <li>Stop all ticket sales immediately</li>
                          <li>Lock in final revenue figures</li>
                          <li>Begin the payout calculation process</li>
                          <li>Prevent any modifications to event settings</li>
                        </ul>
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>Cancel</AlertDialogCancel>
                      <AlertDialogAction
                        onClick={handleEndEvent}
                        className="bg-red-600 hover:bg-red-700"
                        disabled={processing}
                      >
                        {processing ? (
                          <>
                            <Loader2 className="h-4 w-4 animate-spin mr-2" />
                            Processing...
                          </>
                        ) : (
                          'End Event'
                        )}
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              )}
            </div>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              <div className="p-4 bg-gray-50 rounded-lg">
                <p className="text-sm text-gray-500 mb-1">Status</p>
                <p className="text-lg font-semibold">
                  {isEventEnded ? (
                    <span className="text-red-600">Ended</span>
                  ) : (
                    <span className="text-green-600">Active</span>
                  )}
                </p>
              </div>
              
              {payout && (
                <>
                  <div className="p-4 bg-gray-50 rounded-lg">
                    <p className="text-sm text-gray-500 mb-1">Total Revenue</p>
                    <p className="text-lg font-semibold">
                      {new Intl.NumberFormat('en-ZA', {
                        style: 'currency',
                        currency: 'ZAR'
                      }).format(payout.amount)}
                    </p>
                  </div>
                  <div className="p-4 bg-gray-50 rounded-lg">
                    <p className="text-sm text-gray-500 mb-1">Payout Status</p>
                    <div className="flex items-center space-x-2">
                      {payout.status === 'completed' ? (
                        <CheckCircle2 className="h-5 w-5 text-green-600" />
                      ) : payout.status === 'processing' ? (
                        <Loader2 className="h-5 w-5 text-blue-600 animate-spin" />
                      ) : payout.status === 'failed' ? (
                        <XCircle className="h-5 w-5 text-red-600" />
                      ) : (
                        <Clock className="h-5 w-5 text-gray-600" />
                      )}
                      <span className="font-semibold capitalize">{payout.status}</span>
                    </div>
                  </div>
                </>
              )}
            </div>
          </CardContent>
        </Card>

        {!isEventEnded ? (
          <div className="text-center py-12">
            <PiggyBank className="h-16 w-16 mx-auto text-gray-400 mb-4" />
            <h3 className="text-xl font-semibold text-gray-900 mb-2">No Payouts Available Yet</h3>
            <p className="text-gray-600">End your event to view and process payouts.</p>
          </div>
        ) : !payout ? (
          <div className="text-center py-12">
            <AlertTriangle className="h-16 w-16 mx-auto text-amber-500 mb-4" />
            <h3 className="text-xl font-semibold text-gray-900 mb-2">No Payout Information</h3>
            <p className="text-gray-600">There seems to be an issue with the payout information.</p>
          </div>
        ) : null}
      </div>
    </div>
  );
};

export default PayoutsPage;