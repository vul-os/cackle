import * as React from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { CategorySection } from './categories';
import { Calendar, MapPin, Link2, Globe, Image, Info, Bold, Italic, List, ListOrdered, Quote, Link, Eye } from 'lucide-react';
import DatePickerWithRange from '@/components/date-range-picker';
import { supabase } from '@/services/supabaseClient';
import { ImageUploader } from './image-uploader';
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import ReactMarkdown from 'react-markdown';

const MarkdownToolbar = ({ onAction }) => {
  return (
    <div className="flex items-center gap-1 p-1 bg-gray-50 border-b">
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('**', '**')} 
        className="h-8 w-8 p-0"
      >
        <Bold className="h-4 w-4" />
      </Button>
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('*', '*')} 
        className="h-8 w-8 p-0"
      >
        <Italic className="h-4 w-4" />
      </Button>
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('\n- ', '')} 
        className="h-8 w-8 p-0"
      >
        <List className="h-4 w-4" />
      </Button>
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('\n1. ', '')} 
        className="h-8 w-8 p-0"
      >
        <ListOrdered className="h-4 w-4" />
      </Button>
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('\n> ', '')} 
        className="h-8 w-8 p-0"
      >
        <Quote className="h-4 w-4" />
      </Button>
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('[', '](url)')} 
        className="h-8 w-8 p-0"
      >
        <Link className="h-4 w-4" />
      </Button>
    </div>
  );
};

const MarkdownEditor = ({ value, onChange, name, placeholder, minHeight = "200px", disabled }) => {
  const [activeTab, setActiveTab] = React.useState("write");
  const inputClasses = "border-gray-200 hover:border-gray-300 transition-colors bg-white";

  const handleMarkdownAction = (prefix, suffix) => {
    const textarea = document.querySelector(`textarea[name="${name}"]`);
    if (!textarea) return;

    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const text = textarea.value;
    const before = text.substring(0, start);
    const selection = text.substring(start, end);
    const after = text.substring(end);

    const newText = before + prefix + selection + suffix + after;
    onChange(newText);

    // Reset cursor position
    textarea.focus();
    const newCursor = start + prefix.length + selection.length + suffix.length;
    textarea.setSelectionRange(newCursor, newCursor);
  };

  return (
    <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
      <TabsList className="grid w-full grid-cols-2 mb-2">
        <TabsTrigger value="write" className="flex items-center gap-2">
          <Link2 className="h-4 w-4" />
          Write
        </TabsTrigger>
        <TabsTrigger value="preview" className="flex items-center gap-2">
          <Eye className="h-4 w-4" />
          Preview
        </TabsTrigger>
      </TabsList>
      <TabsContent value="write" className="mt-0">
        <div className="border rounded-md">
          <MarkdownToolbar onAction={handleMarkdownAction} />
          <Textarea
            name={name}
            placeholder={placeholder}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            className={`${inputClasses} min-h-[${minHeight}] resize-none border-0 rounded-none rounded-b-md`}
            disabled={disabled}
          />
        </div>
      </TabsContent>
      <TabsContent value="preview" className="mt-0">
        <div className="border rounded-md p-4" style={{ minHeight }}>
          <div className="prose prose-sm max-w-none">
            <ReactMarkdown>{value || '*No content yet*'}</ReactMarkdown>
          </div>
        </div>
      </TabsContent>
    </Tabs>
  );
};

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
            <h2 className="text-sm font-medium">Event Details</h2>
          </div>
          <MarkdownEditor
            name="description"
            value={editForm.description}
            onChange={(value) => handleInputChange('description', value)}
            placeholder="Write a compelling description of your event using Markdown..."
            disabled={isSubmitting}
          />
        </div>

        {/* Information Section */}
        <div className="space-y-4 border-t pt-6">
          <div className="flex items-center gap-2 text-gray-500">
            <Info className="h-4 w-4" />
            <h2 className="text-sm font-medium">Additional Information</h2>
          </div>
          <div className="space-y-4">
            <div className="space-y-2">
              <label className="text-sm font-medium">Event Capacity</label>
              <Input
                type="number"
                placeholder="Enter maximum number of attendees"
                value={editForm.capacity}
                onChange={(e) => handleInputChange('capacity', e.target.value)}
                className={inputClasses}
                disabled={isSubmitting}
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">Age Restrictions</label>
              <Input
                placeholder="e.g., 18+, All Ages, etc."
                value={editForm.age_restriction}
                onChange={(e) => handleInputChange('age_restriction', e.target.value)}
                className={inputClasses}
                disabled={isSubmitting}
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">Special Requirements</label>
              <MarkdownEditor
                name="special_requirements"
                value={editForm.special_requirements}
                onChange={(value) => handleInputChange('special_requirements', value)}
                placeholder="Enter any special requirements or important information for attendees..."
                minHeight="100px"
                disabled={isSubmitting}
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">Contact Information</label>
              <Input
                placeholder="Enter contact details for inquiries"
                value={editForm.contact_info}
                onChange={(e) => handleInputChange('contact_info', e.target.value)}
                className={inputClasses}
                disabled={isSubmitting}
              />
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
};

export default EventDetailsCard;