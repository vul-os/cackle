import { useRef, useState } from 'react';
import { DragDropContext, Draggable, Droppable } from '@hello-pangea/dnd';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog';
import { Image, Loader2, Plus, GripVertical, X } from 'lucide-react';
import { useImageUploader } from './use-image-uploader';

export const ImageUploader = ({ eventId, organizationId, onImagesChange }) => {
  const fileInputRef = useRef(null);
  const [selectedImage, setSelectedImage] = useState(null);
  const [loadingImages, setLoadingImages] = useState({});
  
  const {
    images,
    isUploading,
    signedUrls,
    handleFileSelect,
    handleRemoveImage,
    handleDragEnd
  } = useImageUploader(eventId, organizationId, onImagesChange);

  const handleImageLoad = (imageUrl) => {
    setLoadingImages(prev => ({
      ...prev,
      [imageUrl]: false
    }));
  };

  const handleImageError = (imageUrl) => {
    setLoadingImages(prev => ({
      ...prev,
      [imageUrl]: false
    }));
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => fileInputRef.current?.click()}
          disabled={isUploading}
          className="flex items-center gap-2"
        >
          {isUploading ? (
            <>
              <Loader2 className="h-4 w-4 animate-spin" />
              <span>Uploading...</span>
            </>
          ) : (
            <>
              <Plus className="h-4 w-4" />
              <span>Add Images</span>
            </>
          )}
        </Button>
      </div>

      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        multiple
        className="hidden"
        onChange={(e) => handleFileSelect(Array.from(e.target.files))}
        disabled={isUploading}
      />

      <DragDropContext onDragEnd={handleDragEnd}>
        <Droppable droppableId="images">
          {(provided) => (
            <div
              {...provided.droppableProps}
              ref={provided.innerRef}
              className="space-y-2"
            >
              {images.map((image, index) => (
                <Draggable 
                  key={image.id.toString()} 
                  draggableId={image.id.toString()} 
                  index={index}
                >
                  {(provided) => (
                    <div
                      ref={provided.innerRef}
                      {...provided.draggableProps}
                      className="group relative bg-gray-50 rounded-lg overflow-hidden border border-gray-200 hover:border-gray-300 transition-colors"
                    >
                      <div className="flex items-center p-2">
                        <div {...provided.dragHandleProps} className="px-2 cursor-grab active:cursor-grabbing">
                          <GripVertical className="h-4 w-4 text-gray-400" />
                        </div>
                        <div 
                          className="h-16 w-24 relative rounded overflow-hidden bg-gray-100 cursor-pointer"
                          onClick={() => setSelectedImage(signedUrls[image.image_url])}
                        >
                          {loadingImages[image.image_url] !== false && (
                            <div className="absolute inset-0 flex items-center justify-center bg-gray-50">
                              <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
                            </div>
                          )}
                          <img
                            src={signedUrls[image.image_url]}
                            alt="Event"
                            className="h-full w-full object-cover"
                            onLoad={() => handleImageLoad(image.image_url)}
                            onError={() => handleImageError(image.image_url)}
                          />
                        </div>
                        <div className="flex-1 px-4 truncate text-sm text-gray-600">
                          {image.image_url.split('/').pop()}
                        </div>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="opacity-0 group-hover:opacity-100 transition-opacity"
                          onClick={() => handleRemoveImage(image.id, image.image_url)}
                        >
                          <X className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  )}
                </Draggable>
              ))}
              {provided.placeholder}
            </div>
          )}
        </Droppable>
      </DragDropContext>

      {images.length === 0 && (
        <div className="text-center py-8 border-2 border-dashed border-gray-200 rounded-lg">
          <Image className="h-8 w-8 mx-auto text-gray-400 mb-2" />
          <p className="text-sm text-gray-500">
            No images yet. Click "Add Images" to upload.
          </p>
        </div>
      )}

      <Dialog open={!!selectedImage} onOpenChange={() => setSelectedImage(null)}>
        <DialogContent className="max-w-4xl w-full p-1">
          <DialogTitle className="sr-only">Image Preview</DialogTitle>
          <Button
            className="absolute right-2 top-2 h-8 w-8 p-0"
            variant="ghost"
            onClick={() => setSelectedImage(null)}
          >
            <X className="h-4 w-4" />
            <span className="sr-only">Close</span>
          </Button>
          <div className="relative w-full aspect-video">
            <img
              src={selectedImage}
              alt="Preview"
              className="w-full h-full object-contain"
            />
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default ImageUploader;