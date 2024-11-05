import React, { useState, useContext } from 'react';
import { useNavigate } from 'react-router-dom';
import { useCart } from '@/context/use-cart';
import { supabase } from '@/services/supabaseClient';
import { AuthContext } from '@/context/use-auth';
import Header from '@/pages/visitor/header';
import { Button } from "@/components/ui/button";
import { ArrowLeft } from 'lucide-react';
import { toast } from '@/components/ui/use-toast';
import BillingForm from './billing-form';
import OrderSummary from './order-summary';
import PaymentRedirectPage from './redirect';

const EDGE_FUNCTION_URL = 'REDACTED_SUPABASE_URL/functions/v1/create-order';

const CheckoutPage = () => {
  const navigate = useNavigate();
  const { items, itemsByEvent, total, id: cartId, clearCart } = useCart();
  const { user } = useContext(AuthContext);

  const [isProcessing, setIsProcessing] = useState(false);
  const [isRedirecting, setIsRedirecting] = useState(false);
  const [redirectUrl, setRedirectUrl] = useState(null);
  const [billingDetails, setBillingDetails] = useState({
    email: user?.email || '',
    name: '',
    address: {
      street: '',
      line2: '',
      city: '',
      state: '',
      postalCode: '',
      country: 'ZA'
    }
  });

  const handleInputChange = (e) => {
    const { name, value } = e.target;
    if (name.includes('.')) {
      const [parent, child] = name.split('.');
      setBillingDetails(prev => ({
        ...prev,
        [parent]: {
          ...prev[parent],
          [child]: value
        }
      }));
    } else {
      setBillingDetails(prev => ({
        ...prev,
        [name]: value
      }));
    }
  };

  const validateBillingDetails = () => {
    const required = [
      'email',
      'name',
      'address.street',
      'address.city',
      'address.state',
      'address.postalCode'
    ];

    const missing = required.filter(field => {
      const value = field.includes('.')
        ? billingDetails[field.split('.')[0]][field.split('.')[1]]
        : billingDetails[field];
      return !value || value.trim() === '';
    });

    if (missing.length > 0) {
      const fieldNames = missing.map(field => 
        field.split('.')[1] || field
      ).join(', ');
      throw new Error(`Please fill in all required fields: ${fieldNames}`);
    }

    if (!billingDetails.email.match(/^[^\s@]+@[^\s@]+\.[^\s@]+$/)) {
      throw new Error('Please enter a valid email address');
    }
  };

  const handleCheckout = async () => {
    if (!user) {
      navigate('/login', { state: { returnTo: '/checkout' } });
      return;
    }

    if (!cartId) {
      toast({
        title: "Cart Error",
        description: "No active cart found. Please try again.",
        variant: "destructive",
      });
      return;
    }

    setIsProcessing(true);

    try {
      validateBillingDetails();

      const { data: { session } } = await supabase.auth.getSession();
      
      if (!session) {
        throw new Error('No active session found');
      }

      const response = await fetch(EDGE_FUNCTION_URL, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${session.access_token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          cartId: cartId,
          billingName: billingDetails.name,
          billingEmail: billingDetails.email,
          billingAddress: {
            street: billingDetails.address.street,
            line2: billingDetails.address.line2,
            city: billingDetails.address.city,
            state: billingDetails.address.state,
            postalCode: billingDetails.address.postalCode,
            country: billingDetails.address.country
          }
        })
      });

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        throw new Error(errorData.error || `HTTP error! status: ${response.status}`);
      }

      const checkoutData = await response.json();
      
      if (!checkoutData.success) {
        throw new Error(checkoutData.error || 'Failed to process checkout');
      }

      await clearCart();
      localStorage.setItem('lastOrderId', checkoutData.order_id);
      
      // Instead of immediately redirecting, set the redirect state
      setRedirectUrl(checkoutData.authorization_url);
      setIsRedirecting(true);

    } catch (error) {
      console.error('Checkout failed:', error);
      
      toast({
        title: "Checkout Failed",
        description: error.message || 'An error occurred during checkout. Please try again.',
        variant: "destructive",
      });
    } finally {
      setIsProcessing(false);
    }
  };

  if (isRedirecting && redirectUrl) {
    return <PaymentRedirectPage redirectUrl={redirectUrl} />;
  }

  if (!items?.length) {
    navigate('/cart');
    return null;
  }

  return (
    <>
      <Header />
      <div className="min-h-screen bg-gray-50 dark:bg-slate-900">
        <div className="container mx-auto px-4 py-8">
          <div className="max-w-6xl mx-auto">
            <Button
              variant="ghost"
              onClick={() => navigate('/cart')}
              className="mb-6"
            >
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Cart
            </Button>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
              <div className="space-y-6">
                <BillingForm 
                  billingDetails={billingDetails}
                  handleInputChange={handleInputChange}
                />
              </div>

              <div>
                <OrderSummary
                  itemsByEvent={itemsByEvent}
                  total={total}
                  isProcessing={isProcessing}
                  onCheckout={handleCheckout}
                />
              </div>
            </div>
          </div>
        </div>
      </div>
    </>
  );
};

export default CheckoutPage;