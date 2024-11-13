import React, { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { supabase } from '@/services/supabaseClient';
import {
  Card,
  CardContent,
} from "@/components/ui/card";
import { Star } from 'lucide-react';
import Header from '@/pages/visitor/header';

// Import components
import EventHeader from './header';
import EventQuickInfo from './quick-info';
import ImageSlider from './image-slider';
import ProcessedText from './processed-text';
import InformationSection from './information';
import LocationSection from './location';

const EventDetailsSection = ({ description }) => (
  <Card className="border-none bg-white/5 backdrop-blur-lg hover:bg-white/10 transition-colors duration-300">
    <CardContent className="p-8">
      <h2 className="text-3xl font-bold text-white mb-6 flex items-center gap-3">
        <Star className="h-6 w-6 text-[#ff0437]" />
        Event Details
      </h2>
      <ProcessedText content={description} />
    </CardContent>
  </Card>
);

const EventPage = () => {
  const { id } = useParams();
  const [event, setEvent] = useState(null);
  const [ticketTypes, setTicketTypes] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [currentImage, setCurrentImage] = useState(0);
  const [eventImages, setEventImages] = useState([]);
  const [signedUrls, setSignedUrls] = useState({});

  // Function to get signed URLs for images
  const getSignedUrls = async (imageFiles) => {
    const urls = {};
    for (const image of imageFiles) {
      const { data: { signedUrl }, error } = await supabase
        .storage
        .from('event_documents')
        .createSignedUrl(image.image_url, 3600); // 1 hour expiry

      if (!error) {
        urls[image.image_url] = signedUrl;
      }
    }
    return urls;
  };

  // Fetch event data, tickets, and images
  useEffect(() => {
    const fetchEventData = async () => {
      try {
        const [eventResult, ticketsResult, imagesResult] = await Promise.all([
          supabase
            .from('events')
            .select('*')
            .eq('id', id)
            .single(),
          supabase
            .from('ticket_types')
            .select('*')
            .eq('event_id', id),
          supabase
            .from('event_images')
            .select('*')
            .eq('event_id', id)
            .order('sort_order')
        ]);
        console.log(imagesResult)
        if (eventResult.error) throw eventResult.error;
        if (ticketsResult.error) throw ticketsResult.error;
        if (imagesResult.error) throw imagesResult.error;
        
        // Parse policy info if it's a string
        if (typeof eventResult.data.policy_info === 'string') {
          try {
            eventResult.data.policy_info = JSON.parse(eventResult.data.policy_info);
          } catch (e) {
            console.error('Error parsing policy_info:', e);
          }
        }
        
        setEvent(eventResult.data);
        setTicketTypes(ticketsResult.data);
        setEventImages(imagesResult.data);

        // Get signed URLs for all images
        const urls = await getSignedUrls(imagesResult.data);
        setSignedUrls(urls);
      } catch (err) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    };

    if (id) {
      fetchEventData();
    }
  }, [id]);

  // Refresh signed URLs periodically
  useEffect(() => {
    const refreshUrls = async () => {
      const urls = await getSignedUrls(eventImages);
      setSignedUrls(urls);
    };

    // Refresh every 45 minutes (since URLs expire after 1 hour)
    const interval = setInterval(refreshUrls, 45 * 60 * 1000);
    return () => clearInterval(interval);
  }, [eventImages]);

  // Auto-advance image slider
  useEffect(() => {
    if (eventImages.length === 0) return;
    
    const timer = setInterval(() => {
      setCurrentImage((prevImage) => (prevImage + 1) % eventImages.length);
    }, 5000);
    
    return () => clearInterval(timer);
  }, [eventImages.length]);

  if (loading) return (
    <div className="min-h-screen bg-[#ff0437] flex items-center justify-center">
      <div className="text-white text-xl">Loading amazing events...</div>
    </div>
  );

  if (error) return (
    <div className="min-h-screen bg-[#ff0437] flex items-center justify-center">
      <div className="text-red-500 text-xl">Error: {error}</div>
    </div>
  );

  if (!event) return (
    <div className="min-h-screen bg-[#ff0437] flex items-center justify-center">
      <div className="text-white text-xl">Event not found</div>
    </div>
  );

  // Transform event images into the format expected by ImageSlider
  const sliderImages = eventImages.length > 0
    ? eventImages.map(img => signedUrls[img.image_url])
    : ['/images/racing.jpeg']; // Provide a default image

  return (
    <>
      <Header className="fixed top-0 left-0 right-0 z-50" />
      <div className="flex flex-col min-h-screen bg-[#ff0437] pt-16">
        <div className="relative group h-[60vh]">
          <ImageSlider 
            images={sliderImages} 
            currentImage={currentImage} 
          />
          <EventHeader 
            category={event.category || 'Event'} 
            title={event.title || 'Event Title'} 
          />
        </div>

        <div className="bg-black/30 backdrop-blur-xl shadow-2xl border-t border-white/10">
          <div className="max-w-5xl mx-auto p-8">
            <EventQuickInfo 
              event={event}
              ticketTypes={ticketTypes}
            />
          </div>
        </div>

        <div className="max-w-5xl mx-auto p-8 grid grid-cols-1 md:grid-cols-3 gap-8 my-8">
          <div className="col-span-2 space-y-8">
            <EventDetailsSection description={event.description || 'No description available.'} />
            <LocationSection 
              location={event.venue_address || 'Address unavailable'}
              latitude={event.venue_latitude || 0}
              longitude={event.venue_longitude || 0}
            />
          </div>
          <div className="col-span-1">
            <InformationSection 
              information={event.information}
              policyInfo={event.policy_info} 
            />
          </div>
        </div>
      </div>
    </>
  );
};

export default EventPage;