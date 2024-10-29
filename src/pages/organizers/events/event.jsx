import React, { useEffect, useState, useContext } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { supabase } from '@/services/supabaseClient';
import { AuthContext } from '@/context/use-auth';
import { format, parseISO } from 'date-fns';
import { useToast } from "@/components/ui/use-toast";
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Card, CardContent } from '@/components/ui/card';
import { DateRangePicker } from '@/components/date-range-picker';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Calendar,
  Clock,
  MapPin,
  Link2,
  Tag,
  Save,
  Trash2,
  ArrowLeft,
  CalendarIcon,
  Globe
} from 'lucide-react';

// Category and subcategory options
const CATEGORIES = [
  { id: 'conference', label: 'Conference' },
  { id: 'workshop', label: 'Workshop' },
  { id: 'seminar', label: 'Seminar' },
  { id: 'networking', label: 'Networking' },
  { id: 'social', label: 'Social' },
];

const SUBCATEGORIES = {
  conference: [
    { id: 'tech', label: 'Technology' },
    { id: 'business', label: 'Business' },
    { id: 'academic', label: 'Academic' },
  ],
  workshop: [
    { id: 'hands-on', label: 'Hands-on' },
    { id: 'training', label: 'Training' },
    { id: 'certification', label: 'Certification' },
  ],
  seminar: [
    { id: 'industry', label: 'Industry' },
    { id: 'research', label: 'Research' },
    { id: 'professional', label: 'Professional Development' },
  ],
  networking: [
    { id: 'meetup', label: 'Meetup' },
    { id: 'mixer', label: 'Mixer' },
    { id: 'roundtable', label: 'Roundtable' },
  ],
  social: [
    { id: 'party', label: 'Party' },
    { id: 'celebration', label: 'Celebration' },
    { id: 'gathering', label: 'Gathering' },
  ],
};

const EventPage = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const { activeOrganization } = useContext(AuthContext);
  const { toast } = useToast();
  const [event, setEvent] = useState(null);
  const [loading, setLoading] = useState(true);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [hasChanges, setHasChanges] = useState(false);
  const [editForm, setEditForm] = useState({
    title: '',
    description: '',
    start_time: '',
    end_time: '',
    category: '',
    subcategory: '',
    venue_name: '',
    venue_address: '',
    url: ''
  });
  
  const [dateRange, setDateRange] = useState({
    from: null,
    to: null
  });

  // Get available subcategories based on selected category
  const availableSubcategories = SUBCATEGORIES[editForm.category] || [];

  useEffect(() => {
    if (id) {
      fetchEvent();
    }
  }, [id]);

  useEffect(() => {
    if (event) {
      const hasUpdates = Object.keys(editForm).some(key => editForm[key] !== event[key]);
      setHasChanges(hasUpdates);
    }
  }, [editForm, event]);

  useEffect(() => {
    if (dateRange.from && dateRange.to) {
      const newStartTime = new Date(dateRange.from);
      const newEndTime = new Date(dateRange.to);
      
      setEditForm(prev => ({
        ...prev,
        start_time: newStartTime.toISOString(),
        end_time: newEndTime.toISOString()
      }));
    }
  }, [dateRange]);

  const fetchEvent = async () => {
    try {
      setLoading(true);
      const { data, error } = await supabase
        .from('events')
        .select('*')
        .eq('id', id)
        .single();

      if (error) throw error;
      setEvent(data);
      setEditForm({
        title: data.title,
        description: data.description || '',
        start_time: data.start_time,
        end_time: data.end_time,
        category: data.category || '',
        subcategory: data.subcategory || '',
        venue_name: data.venue_name || '',
        venue_address: data.venue_address || '',
        url: data.url || ''
      });
      
      setDateRange({
        from: parseISO(data.start_time),
        to: parseISO(data.end_time)
      });
    } catch (error) {
      console.error('Error fetching event:', error);
      toast({
        title: "Error",
        description: "Failed to fetch event details",
        variant: "destructive"
      });
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async () => {
    try {
      const { error } = await supabase
        .from('events')
        .delete()
        .eq('id', id);

      if (error) throw error;

      toast({
        title: "Success",
        description: "Event deleted successfully"
      });
      navigate('/events');
    } catch (error) {
      console.error('Error deleting event:', error);
      toast({
        title: "Error",
        description: "Failed to delete event",
        variant: "destructive"
      });
    }
  };

  const handleSave = async () => {
    try {
      const { error } = await supabase
        .from('events')
        .update(editForm)
        .eq('id', id);

      if (error) throw error;

      toast({
        title: "Success",
        description: "Event updated successfully"
      });
      setHasChanges(false);
      fetchEvent();
    } catch (error) {
      console.error('Error updating event:', error);
      toast({
        title: "Error",
        description: "Failed to update event",
        variant: "destructive"
      });
    }
  };

  const handleInputChange = (field, value) => {
    setEditForm(prev => ({
      ...prev,
      [field]: value
    }));
    
    // Reset subcategory when category changes
    if (field === 'category') {
      setEditForm(prev => ({
        ...prev,
        subcategory: ''
      }));
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900"></div>
      </div>
    );
  }

  if (!event) {
    return (
      <div className="flex flex-col items-center justify-center min-h-screen gap-4">
        <div className="text-xl font-semibold text-gray-700">Event not found</div>
        <Button variant="outline" onClick={() => navigate('/events')}>
          Return to Events
        </Button>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-white p-4 md:p-8">
      <div className="max-w-4xl mx-auto">
        {/* Header */}
        <div className="mb-8">
          <Button
            variant="ghost"
            onClick={() => navigate('/events')}
            className="mb-4 hover:bg-gray-100"
          >
            <ArrowLeft className="h-4 w-4 mr-2" />
            Back to Events
          </Button>

          <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
            <div className="flex-1">
              <Input
                value={editForm.title}
                onChange={(e) => handleInputChange('title', e.target.value)}
                className="text-2xl md:text-3xl font-bold text-gray-900 border-transparent hover:border-gray-200 transition-colors bg-transparent p-2 h-auto focus-visible:ring-1"
                placeholder="Event Title"
              />
            </div>
            <div className="flex gap-2">
              {hasChanges && (
                <Button
                  variant="default"
                  onClick={handleSave}
                  className="bg-green-600 hover:bg-green-700 transition-colors"
                >
                  <Save className="h-4 w-4 mr-2" />
                  Save Changes
                </Button>
              )}
              <Button
                variant="destructive"
                onClick={() => setShowDeleteDialog(true)}
                className="hover:bg-red-700 transition-colors"
              >
                <Trash2 className="h-4 w-4 mr-2" />
                Delete
              </Button>
            </div>
          </div>
        </div>

        {/* Event Details */}
        <Card className="shadow-lg border-gray-200/80">
          <CardContent className="space-y-8 pt-6">
            {/* Date and Time Section */}
            <div className="space-y-4">
              <div className="flex items-center gap-2 text-gray-500">
                <Calendar className="h-4 w-4" />
                <h2 className="text-sm font-medium">Date & Time</h2>
              </div>
              <DateRangePicker 
                date={dateRange}
                setDate={setDateRange}
              />
            </div>

            {/* Location Section */}
            <div className="space-y-4 border-t pt-6">
              <div className="flex items-center gap-2 text-gray-500">
                <MapPin className="h-4 w-4" />
                <h2 className="text-sm font-medium">Location</h2>
              </div>
              <div className="space-y-4">
                <Input
                  placeholder="Venue Name"
                  value={editForm.venue_name}
                  onChange={(e) => handleInputChange('venue_name', e.target.value)}
                  className="border-gray-200 hover:border-gray-300 transition-colors bg-white font-medium"
                />
                <Input
                  placeholder="Venue Address"
                  value={editForm.venue_address}
                  onChange={(e) => handleInputChange('venue_address', e.target.value)}
                  className="border-gray-200 hover:border-gray-300 transition-colors bg-white text-gray-600"
                />
              </div>
            </div>

            {/* Category Section */}
            <div className="space-y-4 border-t pt-6">
              <div className="flex items-center gap-2 text-gray-500">
                <Tag className="h-4 w-4" />
                <h2 className="text-sm font-medium">Category</h2>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <Select
                  value={editForm.category}
                  onValueChange={(value) => handleInputChange('category', value)}
                >
                  <SelectTrigger className="border-gray-200 hover:border-gray-300 transition-colors bg-white">
                    <SelectValue placeholder="Select category" />
                  </SelectTrigger>
                  <SelectContent>
                    {CATEGORIES.map((category) => (
                      <SelectItem key={category.id} value={category.id}>
                        {category.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>

                <Select
                  value={editForm.subcategory}
                  onValueChange={(value) => handleInputChange('subcategory', value)}
                  disabled={!editForm.category}
                >
                  <SelectTrigger className="border-gray-200 hover:border-gray-300 transition-colors bg-white">
                    <SelectValue placeholder="Select subcategory" />
                  </SelectTrigger>
                  <SelectContent>
                    {availableSubcategories.map((subcategory) => (
                      <SelectItem key={subcategory.id} value={subcategory.id}>
                        {subcategory.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* URL Section */}
            <div className="space-y-4 border-t pt-6">
              <div className="flex items-center gap-2 text-gray-500">
                <Globe className="h-4 w-4" />
                <h2 className="text-sm font-medium">Event URL</h2>
              </div>
              <Input
                placeholder="https://"
                value={editForm.url}
                onChange={(e) => handleInputChange('url', e.target.value)}
                className="border-gray-200 hover:border-gray-300 transition-colors bg-white text-blue-600"
              />
            </div>

            {/* Description Section */}
            <div className="space-y-4 border-t pt-6">
              <div className="flex items-center gap-2 text-gray-500">
                <Link2 className="h-4 w-4" />
                <h2 className="text-sm font-medium">Description</h2>
              </div>
              <Textarea
                placeholder="Event description"
                value={editForm.description}
                onChange={(e) => handleInputChange('description', e.target.value)}
                className="border-gray-200 hover:border-gray-300 transition-colors bg-white min-h-[200px] resize-none"
              />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Are you sure?</AlertDialogTitle>
            <AlertDialogDescription>
              This action cannot be undone. This will permanently delete the event.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={handleDelete} className="bg-red-600 hover:bg-red-700">
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
};

export default EventPage;