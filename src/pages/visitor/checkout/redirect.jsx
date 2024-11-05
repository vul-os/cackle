import React, { useEffect } from 'react';
import PaymentStatusPage from '@/pages/visitor/payment/status';

export default function PaymentRedirectPage({ redirectUrl }) {
    console.log
  useEffect(() => {
    const timer = setTimeout(() => {
      window.location.href = redirectUrl;
    }, 1500);

    return () => clearTimeout(timer);
  }, [redirectUrl]);

  return <PaymentStatusPage theme="redirecting" />;
}