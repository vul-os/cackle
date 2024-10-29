import React, { useEffect, useState, useContext, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { supabase } from '@/services/supabaseClient';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Plus, Search, SortAsc, Calendar } from 'lucide-react';
import { format } from 'date-fns';
import { AuthContext } from '@/context/use-auth';

const EventsPage = () => {
  const navigate = useNavigate();
  const { activeOrganization } = useContext(AuthContext);
  const [events, setEvents] = useState([]);
  const [filteredEvents, setFilteredEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [sortBy, setSortBy] = useState('start_time');
  const [sortOrder, setSortOrder] = useState('asc');

  const handleEventClick = (eventId) => {
    navigate(`/events/${eventId}`);
  };
  
  useEffect(() => {
    fetchEvents();
  }, [activeOrganization]);

  // Apply search and sort whenever events, searchQuery, or sort parameters change
  useEffect(() => {
    let result = [...events];

    // Apply search filter
    if (searchQuery) {
      result = result.filter(event =>
        event.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
        event.description?.toLowerCase().includes(searchQuery.toLowerCase()) ||
        event.venue_name?.toLowerCase().includes(searchQuery.toLowerCase())
      );
    }

    // Apply sorting
    result.sort((a, b) => {
      let compareA = a[sortBy];
      let compareB = b[sortBy];

      // Handle dates
      if (sortBy === 'start_time') {
        compareA = new Date(compareA);
        compareB = new Date(compareB);
      }
      // Handle strings
      else if (typeof compareA === 'string') {
        compareA = compareA.toLowerCase();
        compareB = compareB.toLowerCase();
      }

      if (compareA < compareB) return sortOrder === 'asc' ? -1 : 1;
      if (compareA > compareB) return sortOrder === 'asc' ? 1 : -1;
      return 0;
    });

    setFilteredEvents(result);
  }, [events, searchQuery, sortBy, sortOrder]);

  const fetchEvents = useCallback(async () => {
    if (!activeOrganization?.id) return;
    
    try {
      setLoading(true);
      const { data, error } = await supabase
        .from('events')
        .select('*')
        .eq('organization_id', activeOrganization.id);
  
      if (error) throw error;
      setEvents(data);
    } catch (error) {
      console.error('Error fetching events:', error);
    } finally {
      setLoading(false);
    }
  }, [activeOrganization?.id]);

  const handleSearch = (e) => {
    setSearchQuery(e.target.value);
  };

  const handleSort = (sortType) => {
    if (sortBy === sortType) {
      // Toggle sort order if clicking the same sort type
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      setSortBy(sortType);
      setSortOrder('asc');
    }
  };

  return (
    <div className="min-h-screen bg-gray-50 p-8">
      {/* Header */}
      <div className="max-w-6xl mx-auto mb-8">
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center space-x-4">
            <Calendar className="h-8 w-8 text-primary" />
            <h1 className="text-3xl font-bold text-gray-900">Events</h1>
          </div>
          <Button onClick={() => window.location.href = '/events/new'}>
            <Plus className="h-4 w-4 mr-2" />
            Create Event
          </Button>
        </div>

        {/* Search and Sort Controls */}
        <div className="flex items-center space-x-4 mb-6">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-gray-400" />
            <Input
              placeholder="Search events..."
              value={searchQuery}
              onChange={handleSearch}
              className="pl-10"
            />
          </div>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline">
                <SortAsc className="h-4 w-4 mr-2" />
                Sort
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <DropdownMenuItem onClick={() => handleSort('title')}>
                By Name
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => handleSort('start_time')}>
                By Date
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => handleSort('category')}>
                By Category
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Events Grid */}
      <div className="max-w-6xl mx-auto">
        {loading ? (
          <div className="text-center py-12">Loading events...</div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {filteredEvents.length === 0 ? (
              <div className="col-span-full text-center py-12 text-gray-500">
                No events found matching your search.
              </div>
            ) : (
              filteredEvents.map((event) => (
                <Card 
                key={event.id} 
                className="hover:shadow-lg transition-shadow cursor-pointer" 
                onClick={() => handleEventClick(event.id)}
                >                  
                  <CardHeader>
                    <CardTitle>{event.title}</CardTitle>
                    <CardDescription>
                      {event.category} {event.subcategory && `• ${event.subcategory}`}
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className="space-y-2">
                      <div className="flex items-center text-sm text-gray-600">
                        <Calendar className="h-4 w-4 mr-2" />
                        {format(new Date(event.start_time), 'PPP')}
                      </div>
                      {event.venue_name && (
                        <p className="text-sm text-gray-600">
                          📍 {event.venue_name}
                        </p>
                      )}
                      {event.description && (
                        <p className="text-sm text-gray-600 line-clamp-2">
                          {event.description}
                        </p>
                      )}
                    </div>
                  </CardContent>
                </Card>
              ))
            )}
          </div>
        )}
      </div>
    </div>
  );
};

export default EventsPage;