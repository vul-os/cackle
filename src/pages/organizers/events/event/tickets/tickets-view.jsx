"use client";

import React, { useEffect, useState, useCallback, memo } from 'react';
import { useParams } from 'react-router-dom';
import { supabase } from '@/services/supabaseClient';
import { format } from 'date-fns';
import { 
  Card, 
  CardContent, 
  CardHeader, 
  CardTitle 
} from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Spinner } from '@/components/ui/spinner';
import { useToast } from "@/components/ui/use-toast";

// Status color mappings
const statusColors = {
  available: "bg-gray-500",
  reserved: "bg-yellow-500",
  sold: "bg-green-500",
  cancelled: "bg-red-500"
};

const scanStatusColors = {
  valid: "bg-green-500",
  invalid: "bg-red-500",
  duplicate: "bg-yellow-500"
};

const Filters = memo(({ filters, onFilterChange }) => (
    <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
      <Input
        placeholder="Search ticket code..."
        value={filters.ticketCode || ''}
        onChange={(e) => onFilterChange('ticketCode', e.target.value)}
      />
      <Select
        value={filters.status || 'all'}
        onValueChange={(value) => onFilterChange('status', value === 'all' ? '' : value)}
      >
        <SelectTrigger>
          <SelectValue placeholder="Filter by status" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All Statuses</SelectItem>
          <SelectItem value="available">Available</SelectItem>
          <SelectItem value="reserved">Reserved</SelectItem>
          <SelectItem value="sold">Sold</SelectItem>
          <SelectItem value="cancelled">Cancelled</SelectItem>
        </SelectContent>
      </Select>
      <Input
        type="date"
        value={filters.startDate || ''}
        onChange={(e) => onFilterChange('startDate', e.target.value)}
      />
      <Input
        type="date"
        value={filters.endDate || ''}
        onChange={(e) => onFilterChange('endDate', e.target.value)}
      />
    </div>
  ));
  
  

const TicketTable = memo(({ tickets, onViewScans }) => (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Ticket Code</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Purchase Time</TableHead>
          <TableHead>Last Updated</TableHead>
          <TableHead>Scans</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {tickets.map((ticket) => (
          <TableRow key={ticket.id} className="cursor-pointer hover:bg-gray-50" onClick={() => onViewScans(ticket)}>
            <TableCell className="font-medium">{ticket.ticket_code}</TableCell>
            <TableCell>
              <Badge className={`${statusColors[ticket.status]} text-white`}>
                {ticket.status}
              </Badge>
            </TableCell>
            <TableCell>{ticket.purchase_time ? format(new Date(ticket.purchase_time), 'PPpp') : '-'}</TableCell>
            <TableCell>{format(new Date(ticket.updated_at), 'PPpp')}</TableCell>
            <TableCell>{ticket.scan_count?.[0]?.count || 0}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  ));

const ScansList = memo(({ scans }) => (
  <div className="space-y-4">
    {scans.map((scan) => (
      <Card key={scan.id}>
        <CardContent className="p-4">
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <div className="text-sm text-gray-500">Scan Time</div>
              <div>{format(new Date(scan.scan_time), 'PPpp')}</div>
            </div>
            <div>
              <div className="text-sm text-gray-500">Type</div>
              <div className="capitalize">{scan.scan_type}</div>
            </div>
            <div>
              <div className="text-sm text-gray-500">Status</div>
              <Badge className={`${scanStatusColors[scan.status]} text-white`}>
                {scan.status}
              </Badge>
            </div>
            <div>
              <div className="text-sm text-gray-500">Location</div>
              <div>{scan.scan_location || '-'}</div>
            </div>
            {scan.notes && (
              <div className="col-span-2 md:col-span-4">
                <div className="text-sm text-gray-500">Notes</div>
                <div>{scan.notes}</div>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    ))}
  </div>
));

// Main component
const TicketsView = () => {
  const { id: eventId } = useParams();
  const { toast } = useToast();
  const [state, setState] = useState({
    tickets: [],
    selectedTicket: null,
    scans: [],
    loading: true,
    filters: {
      ticketCode: '',
      status: '',
      startDate: '',
      endDate: ''
    }
  });

  const fetchTickets = useCallback(async () => {
    try {
      setState(prev => ({ ...prev, loading: true }));
      
      let query = supabase
        .from('tickets')
        .select(`
          *,
          ticket_type:ticket_types(id, name),
          scan_count:ticket_scans(count)
        `)
        .eq('ticket_types.event_id', eventId);

      if (state.filters.ticketCode) {
        query = query.ilike('ticket_code', `%${state.filters.ticketCode}%`);
      }
      if (state.filters.status) {
        query = query.eq('status', state.filters.status);
      }
      if (state.filters.startDate) {
        query = query.gte('created_at', state.filters.startDate);
      }
      if (state.filters.endDate) {
        query = query.lte('created_at', state.filters.endDate);
      }

      const { data, error } = await query;

      if (error) throw error;

      setState(prev => ({
        ...prev,
        tickets: data,
        loading: false
      }));
    } catch (error) {
      console.error('Error fetching tickets:', error);
      toast({
        title: "Error",
        description: "Failed to fetch tickets",
        variant: "destructive"
      });
      setState(prev => ({ ...prev, loading: false }));
    }
  }, [eventId, state.filters, toast]);

  const fetchScans = useCallback(async (ticketId) => {
    try {
      setState(prev => ({ ...prev, loading: true }));
      
      const { data, error } = await supabase
        .from('ticket_scans')
        .select(`
          *,
          scanned_by:profiles(id, full_name)
        `)
        .eq('ticket_id', ticketId)
        .order('scan_time', { ascending: false });

      if (error) throw error;

      setState(prev => ({
        ...prev,
        scans: data,
        loading: false
      }));
    } catch (error) {
      console.error('Error fetching scans:', error);
      toast({
        title: "Error",
        description: "Failed to fetch ticket scans",
        variant: "destructive"
      });
      setState(prev => ({ ...prev, loading: false }));
    }
  }, [toast]);

  useEffect(() => {
    fetchTickets();
  }, [fetchTickets]);

  const handleFilterChange = useCallback((key, value) => {
    setState(prev => ({
      ...prev,
      filters: {
        ...prev.filters,
        [key]: value
      }
    }));
  }, []);

  const handleViewScans = useCallback((ticket) => {
    setState(prev => ({ ...prev, selectedTicket: ticket }));
    fetchScans(ticket.id);
  }, [fetchScans]);

  if (state.loading) return <Spinner />;

  return (
    <>
      <Card className="mb-8">
        <CardHeader>
          <CardTitle>Tickets</CardTitle>
        </CardHeader>
        <CardContent>
          <Filters 
            filters={state.filters}
            onFilterChange={handleFilterChange}
          />
          <TicketTable 
            tickets={state.tickets}
            onViewScans={handleViewScans}
          />
        </CardContent>
      </Card>

      {state.selectedTicket && (
        <Card>
          <CardHeader>
            <CardTitle>
              Scans for Ticket: {state.selectedTicket.ticket_code}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ScansList scans={state.scans} />
          </CardContent>
        </Card>
      )}
    </>
  );
};

export default memo(TicketsView);