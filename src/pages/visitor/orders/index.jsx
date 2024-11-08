import React, { useState, useEffect, useContext } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useOrder } from '@/context/use-order';
import { supabase } from '@/services/supabaseClient';
import { AuthContext } from '@/context/use-auth';
import Header from '@/pages/visitor/header';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
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
  ChevronLeft, 
  Search,
  AlertCircle,
  Calendar,
  CircleUser,
  Ticket,
  Clock,
  Users,
  ArrowUpRight,
  Tag,
  MapPin,
  Filter,
  SortDesc,
  ChevronDown,
  BarChart3,
  TicketIcon,
  Layout
} from 'lucide-react';

const formatCurrency = (amount) => {
  if (amount == null) return 'R0.00';
  const numericAmount = typeof amount === 'string' ? parseFloat(amount) : amount;
  if (isNaN(numericAmount)) return 'R0.00';
  return new Intl.NumberFormat('en-ZA', {
    style: 'currency',
    currency: 'ZAR',
    minimumFractionDigits: 2
  }).format(numericAmount);
};

const formatDate = (dateString) => {
  if (!dateString) return 'N/A';
  try {
    return new Date(dateString).toLocaleDateString('en-ZA', {
      year: 'numeric',
      month: 'long',
      day: 'numeric'
    });
  } catch {
    return 'N/A';
  }
};

const formatTime = (dateString) => {
  if (!dateString) return 'N/A';
  try {
    return new Date(dateString).toLocaleTimeString('en-ZA', {
      hour: '2-digit',
      minute: '2-digit'
    });
  } catch {
    return 'N/A';
  }
};

const ORDER_STATUS = {
  pending: {
    className: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-100',
    icon: Clock
  },
  processing: {
    className: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-100',
    icon: Users
  },
  completed: {
    className: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-100',
    icon: Ticket
  },
  cancelled: {
    className: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-100',
    icon: AlertCircle
  }
};

const LoadingSpinner = () => (
  <div className="flex items-center justify-center min-h-screen">
    <div className="flex flex-col items-center space-y-4">
      <div className="animate-spin rounded-full h-12 w-12 border-4 border-primary border-t-transparent" />
      <p className="text-gray-500 dark:text-gray-400">Loading your orders...</p>
    </div>
  </div>
);

const ErrorDisplay = ({ message }) => (
  <Card className="max-w-2xl mx-auto mt-8">
    <CardContent className="pt-6">
      <div className="flex flex-col items-center space-y-4 text-center">
        <AlertCircle className="h-12 w-12 text-red-500" />
        <div className="space-y-2">
          <h2 className="text-2xl font-semibold">Error Loading Orders</h2>
          <p className="text-gray-500 dark:text-gray-400">{message}</p>
          <Button 
            onClick={() => window.location.reload()}
            variant="outline"
            className="mt-4"
          >
            Try Again
          </Button>
        </div>
      </div>
    </CardContent>
  </Card>
);

const StatsContent = ({ orders, ticketTypes }) => {
  const totalTickets = Object.values(ticketTypes).reduce(
    (sum, { tickets }) => sum + tickets.length,
    0
  );

  return (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-4 p-4 bg-gradient-to-br from-red-50/90 to-white dark:from-red-950/50 dark:to-gray-900 rounded-lg shadow-lg backdrop-blur-sm">
      <div className="p-4 rounded-lg bg-primary/5">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-gray-500">Total Orders</p>
            <h4 className="text-2xl font-bold mt-1">{orders.length}</h4>
          </div>
          <div className="p-3 rounded-full bg-primary/10">
            <Ticket className="h-6 w-6 text-primary" />
          </div>
        </div>
      </div>

      <div className="p-4 rounded-lg bg-green-50 dark:bg-green-900/20">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-gray-500">Active Tickets</p>
            <h4 className="text-2xl font-bold mt-1">{totalTickets}</h4>
          </div>
          <div className="p-3 rounded-full bg-green-100 dark:bg-green-900">
            <Ticket className="h-6 w-6 text-green-600 dark:text-green-400" />
          </div>
        </div>
      </div>

      <div className="p-4 rounded-lg bg-blue-50 dark:bg-blue-900/20">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-gray-500">Total Spent</p>
            <h4 className="text-2xl font-bold mt-1">
              {formatCurrency(orders.reduce((sum, order) => 
                sum + (parseFloat(order.total_amount) || 0), 0))}
            </h4>
          </div>
          <div className="p-3 rounded-full bg-blue-100 dark:bg-blue-900">
            <CircleUser className="h-6 w-6 text-blue-600 dark:text-blue-400" />
          </div>
        </div>
      </div>
    </div>
  );
};

const ValidTicketCard = ({ event, ticketTypes, orders }) => {
  const [isStatsOpen, setIsStatsOpen] = useState(false);

  return (
    <Card className="bg-gradient-to-r from-red-50/80 to-white dark:from-red-950/50 dark:to-gray-900 backdrop-blur-sm hover:shadow-lg transition-all duration-200">
      <CardContent className="p-6">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center space-x-3">
            <div className="p-2 bg-primary/10 rounded-lg">
              <Ticket className="h-6 w-6 text-primary" />
            </div>
            <h3 className="font-semibold text-xl">{event.title}</h3>
          </div>
          <div className="flex items-center space-x-3">
            <button
              onClick={() => setIsStatsOpen(!isStatsOpen)}
              className="p-2 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-full transition-colors"
              aria-label="Toggle stats"
            >
              <BarChart3 className={`h-5 w-5 transform transition-transform duration-300 ${
                isStatsOpen ? 'rotate-180' : ''
              }`} />
            </button>
            <Link 
              to={`/events/${event.id}`}
              className="hover:opacity-70 transition-opacity"
            >
              <ArrowUpRight className="h-5 w-5" />
            </Link>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
          <div className="flex items-center space-x-2">
            <Calendar className="h-4 w-4 text-gray-500" />
            <span className="text-sm">{formatDate(event.start_time)}</span>
          </div>
          <div className="flex items-center space-x-2">
            <Clock className="h-4 w-4 text-gray-500" />
            <span className="text-sm">{formatTime(event.start_time)}</span>
          </div>
          <div className="flex items-center space-x-2">
            <MapPin className="h-4 w-4 text-gray-500" />
            <span className="text-sm truncate">{event.venue_name}</span>
          </div>
        </div>

        <div className="space-y-4">
          <div
            className={`transform-gpu transition-all duration-500 origin-top ${
              isStatsOpen ? 'scale-y-100 opacity-100' : 'scale-y-0 opacity-0 h-0'
            }`}
            style={{
              transformStyle: 'preserve-3d',
              perspective: '1000px',
            }}
          >
            <StatsContent orders={orders} ticketTypes={ticketTypes} />
          </div>

          {Object.values(ticketTypes).map(({ type, tickets }) => (
            <div 
              key={type.id}
              className="p-4 bg-gray-50 dark:bg-gray-900 rounded-lg"
            >
              <div className="flex items-center justify-between mb-3">
                <span className="font-medium">{type.name}</span>
                <span className="text-sm text-gray-500">
                  {tickets.length} ticket{tickets.length !== 1 ? 's' : ''}
                </span>
              </div>
              <div className="flex flex-wrap gap-2">
                {tickets.map((ticket) => (
                  <Link 
                    key={ticket.id}
                    to={`/ticket/${ticket.id}`}
                    className="inline-flex items-center px-3 py-1 rounded-full text-xs font-medium bg-primary/10 text-primary hover:bg-primary/20 transition-colors"
                  >
                    {ticket.ticket_code}
                  </Link>
                ))}
              </div>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
};

const OrdersTable = ({ orders, onViewOrder }) => (
  <div className="overflow-x-auto rounded-lg border border-red-100 dark:border-red-900/30">
    <Table>
      <TableHeader>
        <TableRow className="bg-gradient-to-br from-red-50/80 to-white dark:from-red-950/50 dark:to-gray-900 backdrop-blur-sm">
          <TableHead className="w-[100px]">Order ID</TableHead>
          <TableHead>Date</TableHead>
          <TableHead>Events</TableHead>
          <TableHead>Status</TableHead>
          <TableHead className="text-right">Amount</TableHead>
          <TableHead className="w-[100px]"></TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {orders.map((order) => {
          const StatusIcon = ORDER_STATUS[order.status]?.icon;
          return (
            <TableRow 
              key={order.id}
              className="hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors"
            >
              <TableCell className="font-medium">
                {order.id.slice(0, 8)}...
              </TableCell>
              <TableCell>{formatDate(order.created_at)}</TableCell>
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
                <span className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium ${ORDER_STATUS[order.status]?.className}`}>
                  {StatusIcon && <StatusIcon className="h-3 w-3" />}
                  {order.status.charAt(0).toUpperCase() + order.status.slice(1)}
                </span>
              </TableCell>
              <TableCell className="text-right font-medium">
                {formatCurrency(order.total_amount)}
              </TableCell>
              <TableCell>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onViewOrder(order.id)}
                  className="w-full"
                >
                  View
                  <ChevronRight className="ml-2 h-4 w-4" />
                </Button>
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  </div>
);

export default function OrdersPage() {
  const navigate = useNavigate();
  const { user } = useContext(AuthContext);
  const [orders, setOrders] = useState([]);
  const [validTickets, setValidTickets] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [sortBy, setSortBy] = useState('date_desc');
  
  useEffect(() => {
    const fetchOrdersAndTickets = async () => {
      if (!user) return;

      try {
        setLoading(true);
        
        let orderQuery = supabase
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
          orderQuery = orderQuery.eq('status', statusFilter);
        }

        orderQuery = orderQuery.order(
          sortBy.includes('date') ? 'created_at' : 'total_amount',
          { ascending: sortBy.includes('asc') }
        );

        const ticketQuery = supabase
          .from('tickets')
          .select(`
            *,
            ticket_type:ticket_types (
              *,
              event:events (*)
            )
          `)
          .eq('profile_id', user.id)
          .eq('status', 'valid');

          const [ordersResult, ticketsResult] = await Promise.all([
            orderQuery,
            ticketQuery
          ]);
  
          if (ordersResult.error) throw ordersResult.error;
          if (ticketsResult.error) throw ticketsResult.error;
  
          const ticketsByEvent = ticketsResult.data.reduce((acc, ticket) => {
            if (!ticket.ticket_type?.event) return acc;
            
            const eventId = ticket.ticket_type.event.id;
            if (!acc[eventId]) {
              acc[eventId] = {
                event: ticket.ticket_type.event,
                ticketTypes: {}
              };
            }
  
            const typeId = ticket.ticket_type.id;
            if (!acc[eventId].ticketTypes[typeId]) {
              acc[eventId].ticketTypes[typeId] = {
                type: ticket.ticket_type,
                tickets: []
              };
            }
  
            acc[eventId].ticketTypes[typeId].tickets.push(ticket);
            return acc;
          }, {});
  
          setOrders(ordersResult.data);
          setValidTickets(Object.values(ticketsByEvent));
        } catch (err) {
          setError(err.message);
        } finally {
          setLoading(false);
        }
      };
  
      fetchOrdersAndTickets();
    }, [user, statusFilter, sortBy]);
  
    const filteredOrders = orders.filter(order => {
      const searchLower = searchTerm.toLowerCase();
      return order.order_items?.some(item => 
        item.ticket_type?.event?.title?.toLowerCase().includes(searchLower)
      ) || order.id.toLowerCase().includes(searchLower);
    });
  
    if (loading) return <LoadingSpinner />;
    if (error) return <ErrorDisplay message={error} />;
  
    return (
      <>
        <Header />
        <main className="max-w-7xl mx-auto p-4 pt-20 space-y-8 bg-gradient-to-br from-red-50 via-white to-red-100 dark:from-red-950 dark:via-gray-900 dark:to-red-900 
        [background-image:radial-gradient(at_top_left,rgba(255,0,0,0.1)_0%,transparent_50%),radial-gradient(at_bottom_right,rgba(220,38,38,0.1)_0%,transparent_50%)]">
          {/* Back Button */}
          <div className="flex items-center">
            <button
              onClick={() => navigate('/')}
              className="inline-flex items-center px-3 py-2 text-sm font-medium text-gray-700 hover:text-gray-900 dark:text-gray-200 dark:hover:text-white transition-colors"
            >
              <ChevronLeft className="h-4 w-4 mr-2" />
              Back
            </button>
          </div>
  
          {/* Valid Tickets Section */}
          <section>
            <div className="flex items-center justify-between mb-6">
              <div>
                <h2 className="text-2xl font-bold">Valid Tickets</h2>
                <p className="text-gray-500 mt-1">Access your current valid tickets</p>
              </div>
              <Button 
      variant="outline" 
      size="lg" 
      className="gap-2"
      onClick={() => navigate(`/tickets`)}
    >
      <Layout className="h-5 w-5" />
      See All Tickets
      <ChevronRight className="h-4 w-4" />
    </Button>
            </div>
            <div className="grid gap-6">
              {validTickets.map(({ event, ticketTypes }) => (
                <ValidTicketCard
                  key={event.id}
                  event={event}
                  ticketTypes={ticketTypes}
                  orders={orders}
                />
              ))}
            </div>
          </section>
  
          {/* Orders Section */}
          <section>
            <div className="flex items-center justify-between mb-6">
              <div>
                <h2 className="text-2xl font-bold">Order History</h2>
                <p className="text-gray-500 mt-1">View and manage your orders</p>
              </div>
            </div>
  
            <Card>
              <CardContent className="p-6">
                {/* Filters */}
                <div className="flex flex-col md:flex-row gap-4 mb-6">
                  <div className="flex-1">
                    <div className="relative">
                      <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-500" />
                      <Input
                        placeholder="Search orders..."
                        className="pl-10"
                        value={searchTerm}
                        onChange={(e) => setSearchTerm(e.target.value)}
                      />
                    </div>
                  </div>
                  <div className="flex gap-4">
                    <Select value={statusFilter} onValueChange={setStatusFilter}>
                      <SelectTrigger className="w-[180px] bg-white dark:bg-gray-800">
                        <div className="flex items-center gap-2">
                          <Filter className="h-4 w-4 text-gray-500" />
                          <SelectValue placeholder="Filter by status" />
                        </div>
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
                      <SelectTrigger className="w-[180px] bg-white dark:bg-gray-800">
                        <div className="flex items-center gap-2">
                          <SortDesc className="h-4 w-4 text-gray-500" />
                          <SelectValue placeholder="Sort by" />
                        </div>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="date_desc">Newest First</SelectItem>
                        <SelectItem value="date_asc">Oldest First</SelectItem>
                        <SelectItem value="amount_desc">Highest Amount</SelectItem>
                        <SelectItem value="amount_asc">Lowest Amount</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
  
                {/* Orders Table */}
                {filteredOrders.length === 0 ? (
                  <div className="flex flex-col items-center justify-center py-12 text-center">
                    <div className="p-3 rounded-full bg-gray-100 dark:bg-gray-800 mb-4">
                      <Ticket className="h-6 w-6 text-gray-400" />
                    </div>
                    <h3 className="text-lg font-medium mb-2">No Orders Found</h3>
                    <p className="text-gray-500 dark:text-gray-400 max-w-sm">
                      {searchTerm 
                        ? "No orders match your search criteria. Try adjusting your filters."
                        : "You haven't placed any orders yet. Start browsing events to make your first purchase!"}
                    </p>
                    {!searchTerm && (
                      <Button 
                        variant="outline" 
                        className="mt-4"
                        onClick={() => window.location.href = '/events'}
                      >
                        Browse Events
                      </Button>
                    )}
                  </div>
                ) : (
                  <>
                    <OrdersTable 
                      orders={filteredOrders} 
                      onViewOrder={(orderId) => window.location.href = `/order/${orderId}`}
                    />
                    <div className="mt-4 text-sm text-gray-500 dark:text-gray-400">
                      Showing {filteredOrders.length} of {orders.length} orders
                    </div>
                  </>
                )}
              </CardContent>
            </Card>
          </section>
        </main>
      </>
    );
  }