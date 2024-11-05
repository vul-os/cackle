// checkout.jsx
import React, { useState, useContext } from 'react';
import { useNavigate } from 'react-router-dom';
import { useCart } from '@/context/use-cart';
import { useOrder } from '@/context/use-order';
import { AuthContext } from '@/context/use-auth';
import Header from '@/pages/visitor/header';
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { format } from 'date-fns';
import { ArrowLeft, Clock, MapPin, CreditCard } from 'lucide-react';
import { toast } from '@/components/ui/use-toast';

const CheckoutPage = () => {
  const navigate = useNavigate();
  const { items, itemsByEvent, total } = useCart();
  const { user } = useContext(AuthContext);
  const { createOrder, processCheckout, cancelOrder } = useOrder();

  const [isProcessing, setIsProcessing] = useState(false);
  const [billingDetails, setBillingDetails] = useState({
    email: user?.email || '',
    name: '',
    address: {
      line1: '',
      line2: '',
      city: '',
      state: '',
      postal_code: '',
      country: 'US'
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
      'address.line1',
      'address.city',
      'address.state',
      'address.postal_code'
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

    setIsProcessing(true);
    let orderId;

    try {
      // Validate all required fields
      validateBillingDetails();

      // Create order first
      console.log('Creating order...');
      const order = await createOrder(user.id);
      console.log('Order created:', order);
      
      if (!order?.id) {
        throw new Error('Failed to create order');
      }
      orderId = order.id;

      // Process checkout with billing details
      console.log('Processing checkout...');
      const processedOrder = await processCheckout(billingDetails);
      console.log('Checkout processed:', processedOrder);

      if (processedOrder.status !== 'processing') {
        throw new Error('Order processing failed');
      }

      // Navigate to confirmation
      navigate(`/order-confirmation/${orderId}`);
    } catch (error) {
      console.error('Checkout failed:', error);
      
      // Show error message
      toast({
        title: "Checkout Failed",
        description: error.message || 'An error occurred during checkout. Please try again.',
        variant: "destructive",
      });

      // If order was created but processing failed, attempt to cancel it
      if (orderId) {
        try {
          console.log('Cancelling failed order:', orderId);
          await cancelOrder(orderId);
        } catch (cancelError) {
          console.error('Failed to cancel failed order:', cancelError);
        }
      }
    } finally {
      setIsProcessing(false);
    }
  };

  // Redirect if cart is empty
  if (items.length === 0) {
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
              {/* Billing Details */}
              <div className="space-y-6">
                <Card>
                  <CardHeader>
                    <CardTitle>Billing Details</CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="email">Email</Label>
                      <Input
                        id="email"
                        name="email"
                        type="email"
                        value={billingDetails.email}
                        onChange={handleInputChange}
                        required
                      />
                    </div>
                    
                    <div className="space-y-2">
                      <Label htmlFor="name">Full Name</Label>
                      <Input
                        id="name"
                        name="name"
                        value={billingDetails.name}
                        onChange={handleInputChange}
                        required
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="address.line1">Address</Label>
                      <Input
                        id="address.line1"
                        name="address.line1"
                        value={billingDetails.address.line1}
                        onChange={handleInputChange}
                        required
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="address.line2">Address Line 2 (Optional)</Label>
                      <Input
                        id="address.line2"
                        name="address.line2"
                        value={billingDetails.address.line2}
                        onChange={handleInputChange}
                      />
                    </div>

                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <Label htmlFor="address.city">City</Label>
                        <Input
                          id="address.city"
                          name="address.city"
                          value={billingDetails.address.city}
                          onChange={handleInputChange}
                          required
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="address.state">State</Label>
                        <Input
                          id="address.state"
                          name="address.state"
                          value={billingDetails.address.state}
                          onChange={handleInputChange}
                          required
                        />
                      </div>
                    </div>

                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <Label htmlFor="address.postal_code">Postal Code</Label>
                        <Input
                          id="address.postal_code"
                          name="address.postal_code"
                          value={billingDetails.address.postal_code}
                          onChange={handleInputChange}
                          required
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="address.country">Country</Label>
                        <Select
                          value={billingDetails.address.country}
                          onValueChange={(value) => 
                            handleInputChange({
                              target: { name: 'address.country', value }
                            })
                          }
                        >
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="US">United States</SelectItem>
                            <SelectItem value="CA">Canada</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                    </div>
                  </CardContent>
                </Card>

                {/* Payment Details */}
                <Card>
                  <CardHeader>
                    <CardTitle>Payment Method</CardTitle>
                    <CardDescription>All transactions are secure and encrypted.</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className="h-32 border-2 border-dashed border-gray-200 dark:border-slate-700 rounded-lg flex items-center justify-center">
                      <div className="text-gray-500 dark:text-slate-400 flex items-center gap-2">
                        <CreditCard className="h-5 w-5" />
                        Payment form will be integrated here
                      </div>
                    </div>
                  </CardContent>
                </Card>
              </div>

              {/* Order Summary */}
              <div>
                <Card>
                  <CardHeader>
                    <CardTitle>Order Summary</CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    {Object.entries(itemsByEvent).map(([eventId, eventItems]) => {
                      const event = eventItems[0].event;
                      return (
                        <div key={eventId} className="border-b border-gray-200 dark:border-slate-700 pb-4 last:border-0">
                          <h3 className="font-medium dark:text-white mb-2">{event.title}</h3>
                          <div className="text-sm text-gray-500 dark:text-slate-400 space-y-1 mb-3">
                            <div className="flex items-center gap-2">
                              <Clock className="h-4 w-4" />
                              <span>{format(new Date(event.start_time), 'EEE, MMM d, yyyy h:mm a')}</span>
                            </div>
                            {event.venue_name && (
                              <div className="flex items-center gap-2">
                                <MapPin className="h-4 w-4" />
                                <span>{event.venue_name}</span>
                              </div>
                            )}
                          </div>
                          {eventItems.map((item) => (
                            <div 
                              key={item.ticket_type_id}
                              className="flex justify-between text-sm mb-2"
                            >
                              <span className="dark:text-white">
                                {item.quantity}x {item.ticket_type.name}
                              </span>
                              <span className="dark:text-white">
                                ${(item.quantity * item.unit_price).toFixed(2)}
                              </span>
                            </div>
                          ))}
                        </div>
                      );
                    })}
                  </CardContent>
                  <CardFooter className="flex-col space-y-4">
                    <div className="w-full flex justify-between items-center text-lg font-medium">
                      <span>Total</span>
                      <span>${total.toFixed(2)}</span>
                    </div>
                    <Button
                      className="w-full bg-[#FF4848] text-white hover:bg-red-600"
                      onClick={handleCheckout}
                      disabled={isProcessing}
                    >
                      {isProcessing ? 'Processing...' : `Pay $${total.toFixed(2)}`}
                    </Button>
                  </CardFooter>
                </Card>
              </div>
            </div>
          </div>
        </div>
      </div>
    </>
  );
};

export default CheckoutPage;