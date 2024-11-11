import * as React from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { CategorySection } from './categories';
import { Calendar, MapPin, Link2, Globe, Image } from 'lucide-react';
import DatePickerWithRange from '@/components/date-range-picker';
import { supabase } from '@/services/supabaseClient';
import { ImageUploader } from './image-uploader';

export const EventDetailsCard = ({
  editForm,
  handleInputChange,
  initialData,
  isSubmitting,
  categories,
  availableSubcategories,
  organizationId
}) => {
  const [images, setImages] = React.useState([]);
  const [date, setDate] = React.useState(() => {
    if (editForm?.start_time && editForm?.end_time) {
      return {
        from: new Date(editForm.start_time),
        to: new Date(editForm.end_time)
      };
    }
    return undefined;
  });

  // Fetch images on mount
  React.useEffect(() => {
    const fetchImages = async () => {
      if (!editForm.id) return;
      
      const { data, error } = await supabase
        .from('event_images')
        .select('*')
        .eq('event_id', editForm.id)
        .order('sort_order');
        
      if (!error && data) {
        setImages(data);
      }
    };
    
    fetchImages();
  }, [editForm.id]);

  // Keep date in sync with editForm
  React.useEffect(() => {
    if (editForm?.start_time && editForm?.end_time) {
      setDate({
        from: new Date(editForm.start_time),
        to: new Date(editForm.end_time)
      });
    }
  }, [editForm?.start_time, editForm?.end_time]);

  // Watch for date changes and update form
  React.useEffect(() => {
    if (date?.from) {
      handleInputChange('start_time', date.from.toISOString());
    }
    if (date?.to) {
      handleInputChange('end_time', date.to.toISOString());
    }
  }, [date, handleInputChange]);

  const inputClasses = "border-gray-200 hover:border-gray-300 transition-colors bg-white";

  return (
    <Card className="shadow-lg border-gray-200/80">
      <CardContent className="space-y-8 pt-6">
        {/* Hero Image Section */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 text-gray-500">
              <Image className="h-4 w-4" />
              <h2 className="text-sm font-medium">Event Images</h2>
            </div>
          </div>
          <ImageUploader
            eventId={editForm.id}
            organizationId={organizationId}
            images={images}
            setImages={setImages}
            disabled={isSubmitting}
          />
        </div>

        {/* Date & Time Section */}
        <div className="space-y-4 border-t pt-6">
          <div className="flex items-center gap-2 text-gray-500">
            <Calendar className="h-4 w-4" />
            <h2 className="text-sm font-medium">Date & Time</h2>
          </div>
          <div className="space-y-2">
            <label className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
              Event Period
            </label>
            <DatePickerWithRange 
              date={date}
              setDate={setDate}
              className="w-full"
            />
            {!initialData && (
              <p className="text-sm text-muted-foreground mt-1">
                Please select both start and end dates
              </p>
            )}
          </div>
        </div>

        {/* Location Section */}
        <div className="space-y-4 border-t pt-6">
          <div className="flex items-center gap-2 text-gray-500">
            <MapPin className="h-4 w-4" />
            <h2 className="text-sm font-medium">Location</h2>
          </div>
          <div className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">Venue Name</label>
                <Input
                  placeholder="Enter venue name"
                  value={editForm.venue_name}
                  onChange={(e) => handleInputChange('venue_name', e.target.value)}
                  className={inputClasses}
                  disabled={isSubmitting}
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">Venue Address</label>
                <Input
                  placeholder="Enter venue address"
                  value={editForm.venue_address}
                  onChange={(e) => handleInputChange('venue_address', e.target.value)}
                  className={inputClasses}
                  disabled={isSubmitting}
                />
              </div>
            </div>
          </div>
        </div>

        {/* Categories Section */}
        {categories && (
          <div className="border-t pt-6">
            <CategorySection
              editForm={editForm}
              handleInputChange={handleInputChange}
              categories={categories}
              availableSubcategories={availableSubcategories}
              disabled={isSubmitting}
            />
          </div>
        )}

        {/* Event URL Section */}
        <div className="space-y-4 border-t pt-6">
          <div className="flex items-center gap-2 text-gray-500">
            <Globe className="h-4 w-4" />
            <h2 className="text-sm font-medium">Event URL</h2>
          </div>
          <Input
            placeholder="https://"
            value={editForm.url}
            onChange={(e) => handleInputChange('url', e.target.value)}
            className={`${inputClasses} text-blue-600`}
            disabled={isSubmitting}
          />
        </div>

        {/* Description Section */}
        <div className="space-y-4 border-t pt-6">
          <div className="flex items-center gap-2 text-gray-500">
            <Link2 className="h-4 w-4" />
            <h2 className="text-sm font-medium">Description</h2>
          </div>
          <Textarea
            placeholder="Write a compelling description of your event..."
            value={editForm.description}
            onChange={(e) => handleInputChange('description', e.target.value)}
            className={`${inputClasses} min-h-[200px] resize-none`}
            disabled={isSubmitting}
          />
        </div>
      </CardContent>
    </Card>
  );
};

export default EventDetailsCard;