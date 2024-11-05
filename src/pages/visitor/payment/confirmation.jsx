import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { supabase } from '@/services/supabaseClient';
import { toast } from '@/components/ui/use-toast';
import { CheckCircle2, XCircle, Loader2 } from 'lucide-react';
import PaymentStatusPage from './status';

export default function PaymentConfirmationPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [status, setStatus] = useState('processing'); // 'processing', 'success', 'failed'

  useEffect(() => {
    const verifyPayment = async () => {
      const reference = searchParams.get('reference');
      const { data: { session } } = await supabase.auth.getSession();

      if (!reference) {
        setStatus('failed');
        toast({
          title: "Verification Failed",
          description: "No payment reference found",
          variant: "destructive",
        });
        return;
      }

      try {
        const response = await fetch(
          `REDACTED_SUPABASE_URL/functions/v1/payment-verify?reference=${reference}`,
          {
            headers: {
                'Authorization': `Bearer ${session.access_token}`,
            }
          }
        );

        const data = await response.json();

        if (!response.ok) {
          throw new Error(data.error || 'Payment verification failed');
        }

        setStatus(data.status === 'success' ? 'success' : 'failed');

        if (data.status === 'success') {
          toast({
            title: "Payment Successful",
            description: "Your order has been confirmed",
            variant: "success",
          });
        } else {
          toast({
            title: "Payment Failed",
            description: "There was an issue with your payment",
            variant: "destructive",
          });
        }

      } catch (err) {
        console.error('Payment verification error:', err);
        setStatus('failed');
        toast({
          title: "Verification Error",
          description: err.message || 'Failed to verify payment',
          variant: "destructive",
        });
      }
    };

    verifyPayment();
  }, [searchParams]);

  const statusConfigs = {
    processing: {
      icon: Loader2,
      iconClass: 'text-blue-500 animate-spin',
      title: 'Verifying Payment',
      description: 'Please wait while we confirm your payment...',
      buttonText: 'Please wait...',
      buttonDisabled: true
    },
    success: {
      icon: CheckCircle2,
      iconClass: 'text-green-500',
      title: 'Payment Successful!',
      description: 'Thank you for your purchase. Your order has been confirmed.',
      buttonText: 'View My Orders',
      buttonDisabled: false
    },
    failed: {
      icon: XCircle,
      iconClass: 'text-red-500',
      title: 'Payment Failed',
      description: 'We were unable to process your payment. Please try again.',
      buttonText: 'Back to Orders',
      buttonDisabled: false
    }
  };


  return (
    <PaymentStatusPage 
      theme={status}
      onButtonClick={() => navigate('/orders')}
    />
  );;
}