import React, { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { supabase } from '@/services/supabaseClient';
import { Card, CardContent } from "@/components/ui/card";
import { Star, Calendar, MapPin, Info } from 'lucide-react';
import Header from '@/pages/visitor/header';

import EventHeader from './header';
import EventQuickInfo from './quick-info';
import ImageSlider from './image-slider';
import ProcessedText from './processed-text';
import InformationSection from './information';
import LocationSection from './location';

const LoadingView = () => (
  <div className="min-h-screen bg-[#FFF8F8] dark:bg-[#1A1D24] flex items-center justify-center">
    <div className="text-[#1A1D24] dark:text-white text-xl font-semibold animate-pulse">Creating unforgettable moments...</div>
  </div>
);

const ErrorView = ({ message }) => (
  <div className="min-h-screen bg-[#FFF8F8] dark:bg-[#1A1D24] flex items-center justify-center">
    <div className="text-[#1A1D24] dark:text-white text-xl font-semibold">Error: {message}</div>
  </div>
);

const GradientText = ({ children, className = "" }) => (
  <span className={`text-[#1A1D24] dark:text-white font-semibold ${className}`}>
    {children}
  </span>
);

const EventDetailsSection = ({ description }) => (
  <Card className="border-none bg-white dark:bg-[#0A0C10] shadow-lg dark:shadow-none border border-gray-100 dark:border-[#2A2E36] rounded-xl overflow-hidden">
    <CardContent className="p-8">
      <h2 className="text-3xl font-bold mb-6 flex items-center gap-3">
        <GradientText>Event Details</GradientText>
        <Star className="h-6 w-6 text-[#1A1D24] dark:text-white" />
      </h2>
      <div className="text-[#1A1D24] dark:text-[#E5E7EB]">
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
    <div className="min-h-screen bg-[#FFF8F8] dark:bg-[#1A1D24] text-[#1A1D24] dark:text-white">
      <Header className="fixed top-0 left-0 right-0 z-50 bg-white/80 dark:bg-[#0A0C10]/80 backdrop-blur-xl border-b border-gray-100 dark:border-[#2A2E36]" />
      
      <div className="flex flex-col min-h-screen pt-16">
        <div className="relative group h-[70vh]">
          <ImageSlider 
            images={sliderImages} 
            currentImage={currentImage}
            className="rounded-b-xl overflow-hidden"
          />
          <div className="absolute inset-0 bg-gradient-to-t from-[#FFF8F8]/80 dark:from-[#1A1D24]/80 to-transparent" />
          <EventHeader 
            category={event.category} 
            title={event.title}
            className="absolute bottom-0 left-0 right-0 p-8"
          />
        </div>

        <div className="bg-white dark:bg-[#0A0C10] border-t border-gray-100 dark:border-[#2A2E36] shadow-lg dark:shadow-none transform -translate-y-4">
          <div className="max-w-6xl mx-auto p-8">
            <EventQuickInfo 
              event={event}
              ticketTypes={ticketTypes}
              className="bg-[#FFF8F8] dark:bg-[#1A1D24] rounded-xl p-6 text-[#1A1D24] dark:text-white"
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
              className="bg-white dark:bg-[#0A0C10] border border-gray-100 dark:border-[#2A2E36] rounded-xl shadow-lg dark:shadow-none text-[#1A1D24] dark:text-white"
            />
          </div>
          
          <div className="col-span-1">
            <Card className="border-none bg-white dark:bg-[#0A0C10] shadow-lg dark:shadow-none border border-gray-100 dark:border-[#2A2E36] rounded-xl">
              <CardContent className="p-8">
                <h2 className="text-2xl font-bold mb-6">
                  <GradientText>Additional Information</GradientText>
                </h2>
                <InformationSection 
                  information={event.information}
                  policyInfo={event.policy_info}
                  className="text-[#1A1D24] dark:text-[#E5E7EB]"
                />
              </CardContent>
            </Card>
          </div>
        </div>
      </div>
    </div>
  );
};

export default EventPage;