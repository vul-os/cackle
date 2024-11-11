import { useState, useEffect } from 'react';
import { supabase } from '@/services/supabaseClient';
import { useToast } from '@/components/ui/use-toast';

export const useImageUploader = (eventId, organizationId, onImagesChange) => {
  const { toast } = useToast();
  const [images, setImages] = useState([]);
  const [isUploading, setIsUploading] = useState(false);
  const [signedUrls, setSignedUrls] = useState({});

  // Function to get signed URLs for images
  const getSignedUrls = async (imageFiles) => {
    const urls = {};
    for (const image of imageFiles) {
      const { data: { signedUrl }, error } = await supabase
        .storage
        .from('event_documents')
        .createSignedUrl(image.image_url, 3600);

      if (!error) {
        urls[image.image_url] = signedUrl;
      }
    }
    return urls;
  };

  useEffect(() => {
    const fetchImages = async () => {
      if (!eventId) {
        console.log('No eventId provided');
        return;
      }
      
      console.log('Fetching images for eventId:', eventId);
      
      const { data, error } = await supabase
        .from('event_images')
        .select('*')
        .eq('event_id', eventId)
        .order('sort_order');
        
      console.log('Fetch result:', { data, error });
        
      if (!error && data) {
        setImages(data);
        onImagesChange?.(data);
        
        // Get signed URLs for all images
        const urls = await getSignedUrls(data);
        console.log('Signed URLs:', urls);
        setSignedUrls(urls);
      }
    };
    
    fetchImages();
  }, [eventId, onImagesChange]);

  // Refresh signed URLs periodically
  useEffect(() => {
    const refreshUrls = async () => {
      const urls = await getSignedUrls(images);
      setSignedUrls(urls);
    };

    // Refresh every 45 minutes (since URLs expire after 1 hour)
    const interval = setInterval(refreshUrls, 45 * 60 * 1000);
    return () => clearInterval(interval);
  }, [images]);

  const handleFileSelect = async (files) => {
    if (files.length === 0) return;

    setIsUploading(true);
    const uploadedImages = [...images];
    
    try {
      for (const file of files) {
        const fileExt = file.name.split('.').pop();
        const fileName = `${eventId}/${Date.now()}.${fileExt}`;
        
        // Upload to Storage
        const { error: uploadError } = await supabase.storage
          .from('event_documents')
          .upload(fileName, file);

        if (uploadError) throw uploadError;

        // Save to Database
        const { data, error: dbError } = await supabase
          .from('event_images')
          .insert({
            organization_id: organizationId,
            event_id: eventId,
            image_url: fileName,
            image_type: file.type,
            sort_order: images.length
          })
          .select()
          .single();

        if (dbError) throw dbError;

        // Get signed URL for the new image
        const { data: { signedUrl } } = await supabase
          .storage
          .from('event_documents')
          .createSignedUrl(fileName, 3600);

        setSignedUrls(prev => ({
          ...prev,
          [fileName]: signedUrl
        }));

        uploadedImages.push(data);
      }

      setImages(uploadedImages);
      onImagesChange?.(uploadedImages);

      toast({
        title: "Success",
        description: `${files.length} image${files.length > 1 ? 's' : ''} uploaded successfully`,
      });
    } catch (error) {
      console.error('Upload error:', error);
      toast({
        title: "Error",
        description: "Failed to upload images",
        variant: "destructive"
      });
    } finally {
      setIsUploading(false);
    }
  };

  const handleRemoveImage = async (imageId, imageUrl) => {
    try {
      const fileName = imageUrl.split('/').slice(-2).join('/');
      await supabase.storage
        .from('event_documents')
        .remove([fileName]);

      await supabase
        .from('event_images')
        .delete()
        .eq('id', imageId);

      const updatedImages = images.filter(img => img.id !== imageId);
      setImages(updatedImages);
      onImagesChange?.(updatedImages);

      const newSignedUrls = { ...signedUrls };
      delete newSignedUrls[imageUrl];
      setSignedUrls(newSignedUrls);

      toast({
        title: "Success",
        description: "Image removed successfully",
      });
    } catch (error) {
      console.error('Remove error:', error);
      toast({
        title: "Error",
        description: "Failed to remove image",
        variant: "destructive"
      });
    }
  };

  const handleDragEnd = async (result) => {
    if (!result.destination) return;

    const items = Array.from(images);
    const [reorderedItem] = items.splice(result.source.index, 1);
    items.splice(result.destination.index, 0, reorderedItem);

    setImages(items);
    onImagesChange?.(items);

    try {
      const updates = items.map((item, index) => ({
        id: item.id,
        organization_id: organizationId, // Add this
        sort_order: index,
        image_url: item.image_url,
        event_id: eventId  // Add this too for completeness
      }));

      const { error } = await supabase
        .from('event_images')
        .upsert(updates);

      if (error) throw error;
    } catch (error) {
      console.error('Reorder error:', error);
      toast({
        title: "Error",
        description: "Failed to update image order",
        variant: "destructive"
      });
    }
  };

  return {
    images,
    isUploading,
    signedUrls,
    handleFileSelect,
    handleRemoveImage,
    handleDragEnd
  };
};