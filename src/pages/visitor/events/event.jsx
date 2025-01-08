import React, { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { supabase } from '@/services/supabaseClient';
import { Card, CardContent } from "@/components/ui/card";
import { Star, Calendar, MapPin, Info } from 'lucide-react';
import Header from '@/pages/visitor/header';
import Footer from '@/pages/visitor/landing/footer.jsx';

import EventHeader from './header';
import EventQuickInfo from './quick-info';
import ImageSlider from './image-slider';
import ProcessedText from './processed-text';
import InformationSection from './information';
import LocationSection from './location';

const LoadingView = () => (
  <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center">
    <div className="text-gray-900 dark:text-gray-100 text-xl font-semibold animate-pulse">
      Creating unforgettable moments...
    </div>
  </div>
);

const ErrorView = ({ message }) => (
  <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center">
    <div className="text-gray-900 dark:text-gray-100 text-xl font-semibold">
      Error: {message}
    </div>
  </div>
);

const EventDetailsSection = ({ description }) => (
  <Card className="border-none bg-white dark:bg-gray-800 shadow-lg dark:shadow-none border border-gray-200 dark:border-gray-700 rounded-xl overflow-hidden">
    <CardContent className="p-8">
      <h2 className="text-3xl font-bold mb-6 flex items-center gap-3 text-gray-900 dark:text-gray-100">
        Event Details
        <Star className="h-6 w-6 text-gray-900 dark:text-gray-100" />
      </h2>
      <div className="text-gray-700 dark:text-gray-200">
        <ProcessedText content={description} />
      </div>
    </CardContent>
  </Card>
);

const EventPage = () => {
  const { id } = useParams();
  const [eventData, setEventData] = useState({
    event: null,
    ticketTypes: [],
    loading: true,
    error: null,
    currentImage: 0,
    eventImages: [],
    signedUrls: {}
  });

  useEffect(() => {
    const fetchEventData = async () => {
      try {
        const { data: event, error: eventError } = await supabase
          .from('events')
          .select('*')
          .eq('id', id)
          .single();

        if (eventError) throw eventError;

        const { data: tickets, error: ticketsError } = await supabase
          .from('ticket_types')
          .select('*')
          .eq('event_id', id);

        if (ticketsError) throw ticketsError;

        const { data: images, error: imagesError } = await supabase
          .from('event_images')
          .select('*')
          .eq('event_id', id);

        if (imagesError) throw imagesError;

        const signedUrls = {};
        for (const img of images) {
          const { data: { signedUrl } } = await supabase
            .storage
            .from('event-images')
            .createSignedUrl(img.image_url, 3600);
          signedUrls[img.image_url] = signedUrl;
        }

        setEventData({
          event,
          ticketTypes: tickets,
          loading: false,
          error: null,
          currentImage: 0,
          eventImages: images,
          signedUrls
        });
      } catch (error) {
        setEventData(prev => ({
          ...prev,
          loading: false,
          error: error.message
        }));
      }
    };

    fetchEventData();
  }, [id]);

  const { event, ticketTypes, loading, error, currentImage, eventImages, signedUrls } = eventData;

  if (loading) return <LoadingView />;
  if (error) return <ErrorView message={error} />;
  if (!event) return <ErrorView message="Event not found" />;

  const sliderImages = eventImages.length > 0
    ? eventImages.map(img => signedUrls[img.image_url])
    : ['/images/racing.jpeg'];

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-gray-100">
      <Header className="fixed top-0 left-0 right-0 z-50 bg-white/80 dark:bg-gray-800/80 backdrop-blur-xl border-b border-gray-200 dark:border-gray-700" />
      
      <div className="flex flex-col min-h-screen pt-16">
        <div className="relative group h-[70vh]">
          <ImageSlider 
            images={sliderImages} 
            currentImage={currentImage}
            className="rounded-b-xl overflow-hidden"
          />
          <div className="absolute bottom-0 left-0 right-0 p-8 bg-gradient-to-t from-black via-transparent to-transparent">
            <EventHeader 
              category={event.category} 
              title={event.title}
            />
          </div>
        </div>

        <div className="bg-white dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700 shadow-lg dark:shadow-none transform -translate-y-4">
          <div className="max-w-6xl mx-auto p-8">
            <EventQuickInfo 
              event={event}
              ticketTypes={ticketTypes}
              className="bg-gray-50 dark:bg-gray-900 rounded-xl p-6"
            />
          </div>
        </div>

        <div className="max-w-6xl mx-auto p-8 grid grid-cols-1 md:grid-cols-3 gap-8 my-8">
          <div className="col-span-2 space-y-8">
            <EventDetailsSection description={event.description || 'No description available.'} />
            <LocationSection 
              location={event.venue_address}
              latitude={event.venue_latitude}
              longitude={event.venue_longitude}
              className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-xl shadow-lg dark:shadow-none"
            />
          </div>
          
          <div className="col-span-1">
            <Card className="border-none bg-white dark:bg-gray-800 shadow-lg dark:shadow-none border border-gray-200 dark:border-gray-700 rounded-xl">
              <CardContent className="p-8">
                <h2 className="text-2xl font-bold mb-6 text-gray-900 dark:text-gray-100">
                  Additional Information
                </h2>
                <InformationSection 
                  information={event.information}
                  policyInfo={event.policy_info}
                />
              </CardContent>
            </Card>
          </div>
        </div>

        <Footer />
      </div>
    </div>
  );
};

export default EventPage;