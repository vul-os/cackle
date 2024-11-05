import React, { useState, useEffect, useContext} from 'react';
import { Link } from 'react-router-dom';
import { useOrder } from '@/context/use-order';
import { supabase } from '@/services/supabaseClient';
import { AuthContext } from '@/context/use-auth';
import Header from '@/pages/visitor/header';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { 
  ChevronRight, 
  Search,
  AlertCircle,
  Calendar,
  CircleUser,
  Ticket
} from 'lucide-react';

function formatCurrency(amount) {
  if (amount == null) return 'R0.00';
  
  const numericAmount = typeof amount === 'string' ? parseFloat(amount) : amount;
  
  if (isNaN(numericAmount)) return 'R0.00';
  
  const formatted = new Intl.NumberFormat('en-ZA', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(numericAmount);
  
  return numericAmount < 0 ? `-R${formatted.replace('-', '')}` : `R${formatted}`;
}

function formatDate(dateString) {
  return new Date(dateString).toLocaleDateString('en-ZA', {
    year: 'numeric',
    month: 'long',
    day: 'numeric'
  });
}

const ORDER_STATUS = {
  pending: 'bg-yellow-100 text-yellow-800',
  processing: 'bg-green-100 text-green-800',
  cancelled: 'bg-red-100 text-red-800',
  completed: 'bg-blue-100 text-blue-800'
};

export default function OrdersPage() {
  const { user } = useContext(AuthContext);
  const [orders, setOrders] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [sortBy, setSortBy] = useState('date_desc');
  const [uniqueEvents, setUniqueEvents] = useState([]);
  const [selectedEvent, setSelectedEvent] = useState('all');

  useEffect(() => {
    const fetchOrders = async () => {
      if (!user) return;

      try {
        setLoading(true);
        let query = supabase
          .from('orders')
          .select(`
            *,
            order_items (
              *,
              ticket_type:ticket_types (
                *,
                event:events (*)
              )
            )
          `)
          .eq('profile_id', user.id);

        if (statusFilter !== 'all') {
          query = query.eq('status', statusFilter);
        }

        switch (sortBy) {
          case 'date_asc':
            query = query.order('created_at', { ascending: true });
            break;
          case 'date_desc':
            query = query.order('created_at', { ascending: false });
            break;
          case 'amount_asc':
            query = query.order('total_amount', { ascending: true });
            break;
          case 'amount_desc':
            query = query.order('total_amount', { ascending: false });
            break;
          default:
            query = query.order('created_at', { ascending: false });
        }

        const { data, error: fetchError } = await query;

        if (fetchError) throw fetchError;

        const eventsMap = new Map();
        data.forEach(order => {
          order.order_items?.forEach(item => {
            if (item.ticket_type?.event) {
              const event = item.ticket_type.event;
              if (!eventsMap.has(event.id)) {
                eventsMap.set(event.id, {
                  id: event.id,
                  title: event.title,
                  orderCount: 1,
                  totalTickets: 1
                });
              } else {
                const eventStats = eventsMap.get(event.id);
                eventStats.totalTickets += 1;
                if (order.id !== eventStats.lastOrderId) {
                  eventStats.orderCount += 1;
                  eventStats.lastOrderId = order.id;
                }
              }
            }
          });
        });

        setUniqueEvents(Array.from(eventsMap.values()));
        setOrders(data);
      } catch (err) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    };

    fetchOrders();
  }, [user, statusFilter, sortBy]);

  const filteredOrders = orders.filter(order => {
    const searchLower = searchTerm.toLowerCase();
    const matchesSearch = order.order_items?.some(item => 
      item.ticket_type?.event?.title?.toLowerCase().includes(searchLower)
    ) || order.id.toLowerCase().includes(searchLower);

    const matchesEventFilter = selectedEvent === 'all' || 
      order.order_items?.some(item => 
        item.ticket_type?.event?.id === selectedEvent
      );

    return matchesSearch && matchesEventFilter;
  });

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

  if (error) {
    return (
      <>
        <Header />
        <Card className="max-w-2xl mx-auto mt-8">
          <CardContent className="pt-6">
            <div className="flex flex-col items-center space-y-4 text-center">
              <AlertCircle className="h-12 w-12 text-red-500" />
              <div className="space-y-2">
                <h2 className="text-2xl font-semibold">Error Loading Orders</h2>
                <p className="text-gray-500">{error}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </>
    );
  }

  return (
    <>
      <Header />
      <main className="max-w-7xl mx-auto p-4 pt-20 space-y-6">
        {/* Events Summary Card */}
        <Card>
          <CardHeader>
            <CardTitle>Events Summary</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {uniqueEvents.map(event => (
                <div
                  key={event.id}
                  className="p-4 border rounded-lg bg-white shadow-sm hover:shadow-md transition-shadow"
                >
                  <div className="flex items-center space-x-2 mb-2">
                    <Ticket className="h-5 w-5 text-blue-500" />
                    <h3 className="font-medium text-lg">{event.title}</h3>
                  </div>
                  <div className="text-sm text-gray-600">
                    <p>Orders: {event.orderCount}</p>
                    <p>Total Tickets: {event.totalTickets}</p>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>

        {/* Orders Card */}
        <Card>
          <CardHeader>
            <CardTitle>My Orders</CardTitle>
          </CardHeader>
          <CardContent>
            {/* Filters */}
            <div className="flex flex-col md:flex-row gap-4 mb-6">
              <div className="flex-1">
                <div className="relative">
                  <Search className="absolute left-2 top-2.5 h-4 w-4 text-gray-500" />
                  <Input
                    placeholder="Search orders..."
                    className="pl-8"
                    value={searchTerm}
                    onChange={(e) => setSearchTerm(e.target.value)}
                  />
                </div>
              </div>
              <Select value={selectedEvent} onValueChange={setSelectedEvent}>
                <SelectTrigger className="w-[180px]">
                  <SelectValue placeholder="Filter by event" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Events</SelectItem>
                  {uniqueEvents.map(event => (
                    <SelectItem key={event.id} value={event.id}>
                      {event.title}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select value={statusFilter} onValueChange={setStatusFilter}>
                <SelectTrigger className="w-[180px]">
                  <SelectValue placeholder="Filter by status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Orders</SelectItem>
                  <SelectItem value="pending">Pending</SelectItem>
                  <SelectItem value="processing">Processing</SelectItem>
                  <SelectItem value="completed">Completed</SelectItem>
                  <SelectItem value="cancelled">Cancelled</SelectItem>
                </SelectContent>
              </Select>
              <Select value={sortBy} onValueChange={setSortBy}>
                <SelectTrigger className="w-[180px]">
                  <SelectValue placeholder="Sort by" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="date_desc">Newest First</SelectItem>
                  <SelectItem value="date_asc">Oldest First</SelectItem>
                  <SelectItem value="amount_desc">Highest Amount</SelectItem>
                  <SelectItem value="amount_asc">Lowest Amount</SelectItem>
                </SelectContent>
              </Select>
            </div>

            {/* Orders Table */}
            {filteredOrders.length === 0 ? (
              <div className="text-center py-12">
                <p className="text-gray-500">No orders found</p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Order ID</TableHead>
                      <TableHead>Date</TableHead>
                      <TableHead>Events</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead className="text-right">Amount</TableHead>
                      <TableHead></TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredOrders.map((order) => (
                      <TableRow key={order.id}>
                        <TableCell className="font-medium">
                          {order.id.slice(0, 8)}...
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center">
                            <Calendar className="mr-2 h-4 w-4 text-gray-500" />
                            {formatDate(order.created_at)}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="space-y-1">
                            {order.order_items?.map((item, index) => (
                              <div key={index} className="text-sm">
                                {item.ticket_type?.event?.title}
                              </div>
                            ))}
                          </div>
                        </TableCell>
                        <TableCell>
                          <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${ORDER_STATUS[order.status]}`}>
                            {order.status.charAt(0).toUpperCase() + order.status.slice(1)}
                          </span>
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex items-center justify-end">
                            <CircleUser className="mr-1 h-4 w-4 text-gray-500" />
                            {formatCurrency(order.total_amount)}
                          </div>
                        </TableCell>
                        <TableCell>
                          <Link to={`/order/${order.id}`}>
                            <Button variant="ghost" size="sm">
                              View
                              <ChevronRight className="ml-2 h-4 w-4" />
                            </Button>
                          </Link>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>
      </main>
    </>
  );
}