import React, { useState, useEffect } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { CategorySection } from './categories';
import { Calendar, MapPin, Link2, Globe, Image, Info, Bold, Italic, List, ListOrdered, Quote, Link, Eye, Heading1, Heading2, Heading3, Code, Table, Save, X, Trash2 } from 'lucide-react';
import DatePickerWithRange from '@/components/date-range-picker';
import { supabase } from '@/services/supabaseClient';
import { ImageUploader } from './image-uploader';
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import ReactMarkdown from 'react-markdown';
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

const MarkdownToolbar = ({ onAction }) => {
  return (
    <div className="flex items-center gap-1 p-1 bg-gradient-to-r from-blue-50 to-red-50 border-b">
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('# ', '')} 
        className="h-8 w-8 p-0 hover:bg-blue-100/50"
        title="Heading 1"
      >
        <Heading1 className="h-4 w-4 text-blue-600" />
      </Button>
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('## ', '')} 
        className="h-8 w-8 p-0 hover:bg-blue-100/50"
        title="Heading 2"
      >
        <Heading2 className="h-4 w-4 text-blue-600" />
      </Button>
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('### ', '')} 
        className="h-8 w-8 p-0 hover:bg-blue-100/50"
        title="Heading 3"
      >
        <Heading3 className="h-4 w-4 text-blue-600" />
      </Button>
      <div className="w-px h-4 bg-gradient-to-b from-red-200 to-blue-200 mx-1" />
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('**', '**')} 
        className="h-8 w-8 p-0 hover:bg-blue-100/50"
        title="Bold"
      >
        <Bold className="h-4 w-4 text-blue-600" />
      </Button>
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('*', '*')} 
        className="h-8 w-8 p-0 hover:bg-blue-100/50"
        title="Italic"
      >
        <Italic className="h-4 w-4 text-blue-600" />
      </Button>
      <div className="w-px h-4 bg-gradient-to-b from-red-200 to-blue-200 mx-1" />
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('\n- ', '')} 
        className="h-8 w-8 p-0 hover:bg-blue-100/50"
        title="Unordered List"
      >
        <List className="h-4 w-4 text-blue-600" />
      </Button>
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('\n1. ', '')} 
        className="h-8 w-8 p-0 hover:bg-blue-100/50"
        title="Ordered List"
      >
        <ListOrdered className="h-4 w-4 text-blue-600" />
      </Button>
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('\n> ', '')} 
        className="h-8 w-8 p-0 hover:bg-blue-100/50"
        title="Quote"
      >
        <Quote className="h-4 w-4 text-blue-600" />
      </Button>
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('[', '](url)')} 
        className="h-8 w-8 p-0 hover:bg-blue-100/50"
        title="Link"
      >
        <Link className="h-4 w-4 text-blue-600" />
      </Button>
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('`', '`')} 
        className="h-8 w-8 p-0 hover:bg-blue-100/50"
        title="Inline Code"
      >
        <Code className="h-4 w-4 text-blue-600" />
      </Button>
      <Button 
        variant="ghost" 
        size="sm" 
        onClick={() => onAction('\n| Header 1 | Header 2 |\n| --------- | --------- |\n| Cell 1 | Cell 2 |', '')} 
        className="h-8 w-8 p-0 hover:bg-blue-100/50"
        title="Table"
      >
        <Table className="h-4 w-4 text-blue-600" />
      </Button>
    </div>
  );
};

const MarkdownEditor = ({ value, onChange, name, placeholder, minHeight = "200px", disabled }) => {
  const [activeTab, setActiveTab] = useState("write");
  const inputClasses = "border-blue-200 hover:border-blue-300 transition-colors bg-white/95 shadow-sm";

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

    textarea.focus();
    const newCursor = start + prefix.length + selection.length + suffix.length;
    textarea.setSelectionRange(newCursor, newCursor);
  };

  return (
    <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
      <TabsList className="grid w-full grid-cols-2 mb-2 bg-gradient-to-r from-blue-100 to-red-100">
        <TabsTrigger value="write" className="flex items-center gap-2 data-[state=active]:bg-white">
          <Link2 className="h-4 w-4" />
          Write
        </TabsTrigger>
        <TabsTrigger value="preview" className="flex items-center gap-2 data-[state=active]:bg-white">
          <Eye className="h-4 w-4" />
          Preview
        </TabsTrigger>
      </TabsList>
      <TabsContent value="write" className="mt-0">
        <div className="border rounded-md border-blue-200">
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
        <div className="border rounded-md p-4 border-blue-200 bg-white" style={{ minHeight }}>
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
  isSubmitting = false,
  categories,
  availableSubcategories,
  organizationId,
  handleSave = async (formData) => {
    try {
      // If no handleSave function is provided, save to Supabase directly
      if (!editForm?.id) {
        const { data, error } = await supabase
          .from('events')
          .insert([{ ...formData, organization_id: organizationId }])
          .select()
          .single();
          
        if (error) throw error;
        return data;
      } else {
        const { data, error } = await supabase
          .from('events')
          .update({ ...formData })
          .eq('id', editForm.id)
          .select()
          .single();
          
        if (error) throw error;
        return data;
      }
    } catch (error) {
      console.error('Error in default handleSave:', error);
      throw new Error('Failed to save event. Please try again.');
    }
  },
  onDelete
}) => {
  const [images, setImages] = useState([]);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [localHasChanges, setLocalHasChanges] = useState(false);
  const [previousFormState, setPreviousFormState] = useState(null);
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState(null);
  const [date, setDate] = useState(() => {
    if (editForm?.start_time && editForm?.end_time) {
      return {
        from: new Date(editForm.start_time),
        to: new Date(editForm.end_time)
      };
    }
    return undefined;
  });

  useEffect(() => {
    if (!previousFormState) {
      setPreviousFormState(initialData || {});
      return;
    }

    const hasChanges = Object.keys(editForm || {}).some(key => {
      const prev = previousFormState[key] || '';
      const current = editForm[key] || '';
      return prev !== current;
    });

    setLocalHasChanges(hasChanges);
  }, [editForm, initialData, previousFormState]);

  useEffect(() => {
    const fetchImages = async () => {
      if (!editForm?.id) return;
      
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
  }, [editForm?.id]);

  useEffect(() => {
    if (editForm?.start_time && editForm?.end_time) {
      setDate({
        from: new Date(editForm.start_time),
        to: new Date(editForm.end_time)
      });
    }
  }, [editForm?.start_time, editForm?.end_time]);

  useEffect(() => {
    if (date?.from && handleInputChange) {
      handleInputChange('start_time', date.from.toISOString());
    }
    if (date?.to && handleInputChange) {
      handleInputChange('end_time', date.to.toISOString());
    }
  }, [date, handleInputChange]);

  const handleDeleteClick = () => {
    setShowDeleteDialog(true);
  };

  const handleDeleteConfirm = () => {
    setShowDeleteDialog(false);
    if (typeof onDelete === 'function') {
      onDelete();
    }
  };

  const handleLocalSave = async () => {
    if (isSaving) return;
    
    try {
      setIsSaving(true);
      setSaveError(null);
      await handleSave(editForm);
      setPreviousFormState(editForm || {});
      setLocalHasChanges(false);
    } catch (error) {
      console.error('Error saving changes:', error);
      setSaveError(error?.message || 'Failed to save changes. Please try again.');
    } finally {
      setIsSaving(false);
    }
  };

  const inputClasses = "border-blue-200 hover:border-blue-300 transition-colors bg-white/95 focus:ring-2 focus:ring-blue-200";
  const sectionClasses = "space-y-4 border-t border-gradient-to-r from-red-100 to-blue-100 pt-6";
  const headerClasses = "flex items-center gap-2 text-blue-600";
  const iconClasses = "h-4 w-4 text-red-500";
  const labelClasses = "text-sm font-medium text-gray-700";

  return (
    <>
       <Card className="shadow-lg border-blue-200/80 bg-gradient-to-br from-white to-blue-50 relative">
        <div className="absolute top-4 right-4 flex gap-2">
          <Button
            variant="ghost"
            onClick={handleLocalSave}
            disabled={isSubmitting || !localHasChanges || isSaving}
            className={`h-8 w-8 p-0 transition-colors ${
              localHasChanges && !isSubmitting && !isSaving
                ? 'text-blck-600 hover:text-black-700 hover:bg-black-50'
                : 'text-gray-400'
            }`}
            title={isSaving ? 'Saving...' : 'Save Changes'}
          >
            <Save className={`h-4 w-4 ${isSaving ? 'animate-spin' : ''}`} />
          </Button>
          <Button
            variant="ghost"
            onClick={handleDeleteClick}
            disabled={isSubmitting || !editForm?.id || isSaving}
            className="h-8 w-8 p-0 text-red-600 hover:text-red-700 hover:bg-red-50 transition-colors"
            title="Delete Event"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>

        {saveError && (
          <div className="absolute top-16 right-4 bg-red-100 border border-red-400 text-red-700 px-4 py-2 rounded-md">
            {saveError}
          </div>
        )}

        <CardContent className="space-y-8 pt-6">
          {/* Hero Image Section */}
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div className={headerClasses}>
                <Image className={iconClasses} />
                <h2 className={labelClasses}>Event Images</h2></div>
            </div>
            <ImageUploader
              eventId={editForm?.id}
              organizationId={organizationId}
              images={images}
              setImages={setImages}
              disabled={isSubmitting}
            />
          </div>

          {/* Date & Time Section */}
          <div className={sectionClasses}>
            <div className={headerClasses}>
              <Calendar className={iconClasses} />
              <h2 className={labelClasses}>Date & Time</h2>
            </div>
            <div className="space-y-2">
              <label className={labelClasses}>Event Period</label>
              <DatePickerWithRange 
                date={date}
                setDate={setDate}
                className="w-full"
              />
              {!initialData && (
                <p className="text-sm text-blue-600 mt-1">
                  Please select both start and end dates
                </p>
              )}
            </div>
          </div>

          {/* Location Section */}
          <div className={sectionClasses}>
            <div className={headerClasses}>
              <MapPin className={iconClasses} />
              <h2 className={labelClasses}>Location</h2>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <label className={labelClasses}>Venue Name</label>
                <Input
                  placeholder="Enter venue name"
                  value={editForm?.venue_name || ''}
                  onChange={(e) => handleInputChange?.('venue_name', e.target.value)}
                  className={inputClasses}
                  disabled={isSubmitting}
                />
              </div>
              <div className="space-y-2">
                <label className={labelClasses}>Venue Address</label>
                <Input
                  placeholder="Enter venue address"
                  value={editForm?.venue_address || ''}
                  onChange={(e) => handleInputChange?.('venue_address', e.target.value)}
                  className={inputClasses}
                  disabled={isSubmitting}
                />
              </div>
            </div>
          </div>

          {/* Categories Section */}
          {categories && (
            <div className={sectionClasses}>
              <CategorySection
                editForm={editForm || {}}
                handleInputChange={handleInputChange}
                categories={categories}
                availableSubcategories={availableSubcategories}
                disabled={isSubmitting}
              />
            </div>
          )}

          {/* Event URL Section */}
          <div className={sectionClasses}>
            <div className={headerClasses}>
              <Globe className={iconClasses} />
              <h2 className={labelClasses}>Event URL</h2>
            </div>
            <Input
              placeholder="https://"
              value={editForm?.url || ''}
              onChange={(e) => handleInputChange?.('url', e.target.value)}
              className={`${inputClasses} text-blue-600`}
              disabled={isSubmitting}
            />
          </div>

          {/* Description Section */}
          <div className={sectionClasses}>
            <div className={headerClasses}>
              <Link2 className={iconClasses} />
              <h2 className={labelClasses}>Event Details</h2>
            </div>
            <MarkdownEditor
              name="description"
              value={editForm?.description || ''}
              onChange={(value) => handleInputChange?.('description', value)}
              placeholder="Write a compelling description of your event using Markdown..."
              minHeight="300px"
              disabled={isSubmitting}
            />
          </div>

          {/* Information Section */}
          <div className={sectionClasses}>
            <div className={headerClasses}>
              <Info className={iconClasses} />
              <h2 className={labelClasses}>Information</h2>
            </div>
            <MarkdownEditor
              name="information"
              value={editForm?.information || ''}
              onChange={(value) => handleInputChange?.('information', value)}
              placeholder="Enter any special requirements or important information for attendees..."
              minHeight="300px"
              disabled={isSubmitting}
            />
          </div>
        </CardContent>
      </Card>

      <AlertDialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Are you sure you want to delete this event?</AlertDialogTitle>
            <AlertDialogDescription>
              This action cannot be undone. This will permanently delete the event and all associated data.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={handleDeleteConfirm} className="bg-red-600 hover:bg-red-700">
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
};

export default EventDetailsCard;