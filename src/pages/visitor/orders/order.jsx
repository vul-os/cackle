import React, { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useOrder } from '@/context/use-order';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Check, ChevronLeft, AlertCircle, CreditCard } from 'lucide-react';
import Header from '@/pages/visitor/header';

function formatCurrency(amount) {
  return new Intl.NumberFormat('en-ZA', {
    style: 'currency',
    currency: 'ZAR',
  }).format(amount);
}

function formatAddress(address) {
  if (!address) return 'No address provided';
  
  const parts = [];
  
  if (address.line1) parts.push(address.line1);
  if (address.line2) parts.push(address.line2);
  
  const cityStateLine = [
    address.city,
    address.state,
    address.postal_code
  ]
    .filter(Boolean)
    .join(', ');
  
  if (cityStateLine) parts.push(cityStateLine);
  if (address.country) parts.push(address.country);
  
  return parts.join('\n');
}

function getPaymentStatusColor(status) {
  switch (status) {
    case 'completed':
      return 'bg-green-100 text-green-800';
    case 'pending':
      return 'bg-yellow-100 text-yellow-800';
    case 'failed':
      return 'bg-red-100 text-red-800';
    default:
      return 'bg-gray-100 text-gray-800';
  }
}

export default function OrderPage() {
  const { id } = useParams();
  const { getOrder } = useOrder();
  const [order, setOrder] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
   
  useEffect(() => {
    const loadOrder = async () => {
      try {
        setLoading(true);
        const orderData = await getOrder(id);
        setOrder(orderData);
      } catch (err) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    };

    loadOrder();
  }, [id, getOrder]);

  return (
    <>
      <Header />
      <main>
        {loading ? (
          <div className="flex items-center justify-center min-h-[400px]">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900" />
          </div>
        ) : error ? (
          <Card className="max-w-2xl mx-auto mt-8">
            <CardContent className="pt-6">
              <div className="flex flex-col items-center space-y-4 text-center">
                <AlertCircle className="h-12 w-12 text-red-500" />
                <div className="space-y-2">
                  <h2 className="text-2xl font-semibold">Error Loading Order</h2>
                  <p className="text-gray-500">{error}</p>
                </div>
                <Link to="/orders">
                  <Button variant="outline" className="mt-4">
                    <ChevronLeft className="mr-2 h-4 w-4" />
                    Back to Orders
                  </Button>
                </Link>
              </div>
            </CardContent>
          </Card>
        ) : !order ? (
          <Card className="max-w-2xl mx-auto mt-8">
            <CardContent className="pt-6">
              <div className="flex flex-col items-center space-y-4 text-center">
                <AlertCircle className="h-12 w-12 text-yellow-500" />
                <div className="space-y-2">
                  <h2 className="text-2xl font-semibold">Order Not Found</h2>
                  <p className="text-gray-500">We couldn't find the order you're looking for.</p>
                </div>
                <Link to="/orders">
                  <Button variant="outline" className="mt-4">
                    <ChevronLeft className="mr-2 h-4 w-4" />
                    Back to Orders
                  </Button>
                </Link>
              </div>
            </CardContent>
          </Card>
        ) : (
          <div className="max-w-4xl mx-auto p-4 pt-20 space-y-6">
            <Card>
              <CardHeader className="space-y-1">
                <div className="flex items-center justify-between">
                  <CardTitle className="text-2xl">Order Confirmation</CardTitle>
                  <div className="flex items-center space-x-2">
                    <span className={`inline-flex items-center px-3 py-1 rounded-full text-sm font-medium ${getPaymentStatusColor(order.status)}`}>
                      {order.status === 'completed' && <Check className="mr-2 h-4 w-4" />}
                      {order.status.charAt(0).toUpperCase() + order.status.slice(1)}
                    </span>
                  </div>
                </div>
                <p className="text-sm text-gray-500">Order ID: {order.id}</p>
              </CardHeader>
              
              <CardContent className="space-y-6">
                {/* Order Summary */}
                <div className="space-y-4">
                  <h3 className="font-semibold text-lg">Order Summary</h3>
                  <div className="divide-y">
                    {order.order_items?.map((item) => (
                      <div key={item.id} className="py-4 flex justify-between">
                        <div className="space-y-1">
                          <p className="font-medium">{item.ticket_type.event.title}</p>
                          <p className="text-sm text-gray-500">
                            {item.ticket_type.name} × {item.quantity}
                          </p>
                        </div>
                        <p className="font-medium">
                          {formatCurrency(item.subtotal)}
                        </p>
                      </div>
                    ))}
                  </div>
                  
                  {/* Subtotal, Discounts, and Total */}
                  <div className="pt-4 border-t space-y-2">
                    <div className="flex justify-between items-center text-sm">
                      <span className="text-gray-500">Subtotal</span>
                      <span>{formatCurrency(order.total_amount)}</span>
                    </div>
                    {order.order_discounts?.map(discount => (
                      <div key={discount.id} className="flex justify-between items-center text-sm">
                        <span className="text-gray-500">Discount ({discount.discount.code})</span>
                        <span className="text-red-600">-{formatCurrency(discount.amount_saved)}</span>
                      </div>
                    ))}
                    <div className="flex justify-between items-center pt-2 border-t">
                      <span className="font-semibold">Total</span>
                      <span className="font-semibold text-lg">
                        {formatCurrency(order.total_amount)}
                      </span>
                    </div>
                  </div>
                </div>

                {/* Payment Information */}
                <div className="space-y-4">
                  <h3 className="font-semibold text-lg">Payment Information</h3>
                  <div className="bg-gray-50 p-4 rounded-lg space-y-3">
                    <div className="flex items-center">
                      <CreditCard className="h-5 w-5 text-gray-500 mr-2" />
                      <span className="font-medium">Payment Details</span>
                    </div>
                    <div className="grid gap-2 text-sm">
                      <p><span className="text-gray-500">Payment Method:</span> {order.payment_provider?.charAt(0).toUpperCase() + order.payment_provider?.slice(1) || 'Not specified'}</p>
                      <p><span className="text-gray-500">Payment Status:</span> <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium ${getPaymentStatusColor(order.status)}`}>{order.status}</span></p>
                      {order.payment_intent_id && (
                        <p><span className="text-gray-500">Transaction ID:</span> {order.payment_intent_id}</p>
                      )}
                      {order.refund_status !== 'none' && (
                        <p><span className="text-gray-500">Refund Status:</span> {order.refund_status}</p>
                      )}
                    </div>
                  </div>
                </div>

                {/* Billing Details */}
                <div className="space-y-4">
                  <h3 className="font-semibold text-lg">Billing Details</h3>
                  <div className="grid gap-2 text-sm">
                    <p><span className="text-gray-500">Name:</span> {order.billing_name}</p>
                    <p><span className="text-gray-500">Email:</span> {order.billing_email}</p>
                    <div>
                      <span className="text-gray-500">Address:</span>
                      <div className="mt-1 whitespace-pre-line pl-4">
                        {formatAddress(order.billing_address)}
                      </div>
                    </div>
                  </div>
                </div>

                {/* Navigation */}
                <div className="flex justify-between items-center pt-6">
                  <Link to="/orders">
                    <Button variant="outline">
                      <ChevronLeft className="mr-2 h-4 w-4" />
                      Back to Orders
                    </Button>
                  </Link>
                  
                  {order.status === 'completed' && (
                    <Button variant="outline" onClick={() => window.print()}>
                      Print Receipt
                    </Button>
                  )}
                </div>
              </CardContent>
            </Card>
          </div>
        )}
      </main>
    </>
  );
}