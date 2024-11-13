import React, { useState, useEffect } from 'react';
import { supabase } from '@/services/supabaseClient';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Download, Search } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { format } from 'date-fns';

const AttendeesPage = ({ eventId }) => {
  const [attendees, setAttendees] = useState([]);
  const [loading, setLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState("");
  const [filterStatus, setFilterStatus] = useState("all");
  const [stats, setStats] = useState({
    totalAttendees: 0,
    totalRevenue: 0,
    remainingTickets: 0
  });

  // Fetch attendees and stats
  useEffect(() => {
    const fetchData = async () => {
      try {
        // Fetch tickets with related data
        const { data: ticketData, error: ticketError } = await supabase
          .from('tickets')
          .select(`
            id,
            status,
            purchase_time,
            profiles(id, full_name, email),
            ticket_type:ticket_types(
              id,
              name,
              price
            ),
            order:orders(
              id,
              status,
              total_amount
            )
          `)
          .eq('event_id', eventId);

        if (ticketError) throw ticketError;

        // Transform the data
        const transformedAttendees = ticketData.map(ticket => ({
          id: ticket.id,
          name: ticket.profiles?.full_name || 'N/A',
          email: ticket.profiles?.email || 'N/A',
          ticketType: ticket.ticket_type?.name || 'N/A',
          purchaseDate: ticket.purchase_time ? format(new Date(ticket.purchase_time), 'yyyy-MM-dd') : 'N/A',
          status: ticket.status,
          amount: ticket.ticket_type?.price ? `$${ticket.ticket_type.price.toFixed(2)}` : 'N/A'
        }));

        setAttendees(transformedAttendees);

        // Fetch statistics
        const { data: statsData, error: statsError } = await supabase
          .from('ticket_types')
          .select(`
            quantity_total,
            tickets(count),
            price
          `)
          .eq('event_id', eventId);

        if (statsError) throw statsError;

        const totalAttendees = ticketData.length;
        const totalRevenue = ticketData.reduce((sum, ticket) => 
          sum + (ticket.ticket_type?.price || 0), 0);
        const totalCapacity = statsData.reduce((sum, type) => 
          sum + (type.quantity_total || 0), 0);
        const remainingTickets = totalCapacity - totalAttendees;

        setStats({
          totalAttendees,
          totalRevenue,
          remainingTickets
        });

        setLoading(false);
      } catch (error) {
        console.error('Error fetching data:', error);
        setLoading(false);
      }
    };

    fetchData();
  }, [eventId]);

  // Filter attendees based on search and status
  const filteredAttendees = attendees.filter(attendee => {
    const matchesSearch = searchQuery === "" ||
      attendee.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      attendee.email.toLowerCase().includes(searchQuery.toLowerCase());
    
    const matchesStatus = filterStatus === "all" ||
      attendee.status.toLowerCase() === filterStatus.toLowerCase();
    
    return matchesSearch && matchesStatus;
  });

  // Handle CSV export
  const exportToCsv = () => {
    const headers = ['Name', 'Email', 'Ticket Type', 'Purchase Date', 'Status', 'Amount'];
    const csvData = filteredAttendees.map(attendee => [
      attendee.name,
      attendee.email,
      attendee.ticketType,
      attendee.purchaseDate,
      attendee.status,
      attendee.amount
    ]);

    const csvContent = [
      headers.join(','),
      ...csvData.map(row => row.join(','))
    ].join('\n');

    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const link = document.createElement('a');
    link.href = URL.createObjectURL(blob);
    link.download = `attendees_${format(new Date(), 'yyyy-MM-dd')}.csv`;
    link.click();
  };

  if (loading) {
    return <div className="p-6">Loading...</div>;
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-3xl font-bold">Event Attendees</h1>
        <Button 
          variant="outline"
          onClick={exportToCsv}
        >
          <Download className="h-4 w-4 mr-2" />
          Export CSV
        </Button>
      </div>

      <div className="grid gap-6 mb-6">
        <Card>
          <CardHeader>
            <CardTitle>Attendance Overview</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="p-4 bg-gray-50 rounded-lg">
                <div className="text-sm text-gray-500">Total Attendees</div>
                <div className="text-2xl font-bold">{stats.totalAttendees}</div>
              </div>
              <div className="p-4 bg-gray-50 rounded-lg">
                <div className="text-sm text-gray-500">Total Revenue</div>
                <div className="text-2xl font-bold">${stats.totalRevenue.toFixed(2)}</div>
              </div>
              <div className="p-4 bg-gray-50 rounded-lg">
                <div className="text-sm text-gray-500">Tickets Remaining</div>
                <div className="text-2xl font-bold">{stats.remainingTickets}</div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="pt-6">
            <div className="flex flex-col md:flex-row gap-4 mb-6">
              <div className="flex-1">
                <div className="relative">
                  <Search className="absolute left-2 top-2.5 h-4 w-4 text-gray-500" />
                  <Input
                    placeholder="Search by name or email..."
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    className="pl-8"
                  />
                </div>
              </div>
              <Select
                value={filterStatus}
                onValueChange={setFilterStatus}
              >
                <SelectTrigger className="w-[180px]">
                  <SelectValue placeholder="Filter by status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Statuses</SelectItem>
                  <SelectItem value="active">Active</SelectItem>
                  <SelectItem value="used">Used</SelectItem>
                  <SelectItem value="cancelled">Cancelled</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Email</TableHead>
                  <TableHead>Ticket Type</TableHead>
                  <TableHead>Purchase Date</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Amount</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredAttendees.map((attendee) => (
                  <TableRow key={attendee.id}>
                    <TableCell className="font-medium">{attendee.name}</TableCell>
                    <TableCell>{attendee.email}</TableCell>
                    <TableCell>{attendee.ticketType}</TableCell>
                    <TableCell>{attendee.purchaseDate}</TableCell>
                    <TableCell>
                      <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium
                        ${attendee.status === 'active' ? 'bg-green-100 text-green-800' : 
                          attendee.status === 'used' ? 'bg-blue-100 text-blue-800' : 
                          'bg-red-100 text-red-800'}`}>
                        {attendee.status}
                      </span>
                    </TableCell>
                    <TableCell className="text-right">{attendee.amount}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </div>
  );
};

export default AttendeesPage;