import React from 'react';
import { Heart, Share2 } from 'lucide-react';
import { Button } from "@/components/ui/button";

const EventHeader = ({ category, title }) => {
  const handleShare = () => {
    if (navigator.share) {
      navigator.share({
        title: title,
        url: window.location.href,
      }).catch(console.error);
    } else {
      // Fallback to copying to clipboard
      navigator.clipboard.writeText(window.location.href)
        .then(() => {
          // You might want to show a toast notification here
          console.log('Link copied to clipboard');
        })
        .catch(console.error);
    }
  };

  const handleSave = () => {
    // Implement save functionality
    // This could be integrated with your backend to save to user's favorites
    console.log('Save event:', title);
  };

  return (
    <div className="absolute bottom-0 left-0 right-0 p-12 bg-gradient-to-t from-gray-900/90 dark:from-black/90 via-gray-900/50 dark:via-black/50 to-transparent">
      <div className="max-w-5xl mx-auto">
        <div className="space-y-6 transform translate-y-8 transition-transform duration-300 group-hover:translate-y-0">
          <div className="flex flex-wrap gap-4 items-center">
            <span className="inline-block bg-gradient-to-r from-[#880424] to-[#660318] text-gray-100 dark:text-white px-6 py-2 rounded-full text-sm font-medium backdrop-blur-sm">
              {category}
            </span>
            <div className="ml-auto flex gap-4">
              <Button
                variant="outline"
                className="bg-white/10 dark:bg-black/20 backdrop-blur-md border-white/20 dark:border-white/10 
                  text-gray-100 hover:text-white dark:text-white 
                  hover:bg-white/20 dark:hover:bg-white/10 
                  transition-colors"
                onClick={handleSave}
              >
                <Heart className="h-5 w-5 mr-2" />
                Save Event
              </Button>
              <Button
                variant="outline"
                className="bg-white/10 dark:bg-black/20 backdrop-blur-md border-white/20 dark:border-white/10 
                  text-gray-100 hover:text-white dark:text-white 
                  hover:bg-white/20 dark:hover:bg-white/10 
                  transition-colors"
                onClick={handleShare}
              >
                <Share2 className="h-5 w-5 mr-2" />
                Share
              </Button>
            </div>
          </div>
          
          <h1 className="text-6xl md:text-7xl font-bold text-gray-100 dark:text-white mb-6 drop-shadow-lg">
            {title}
          </h1>
        </div>
      </div>
    </div>
  );
};

export default EventHeader;