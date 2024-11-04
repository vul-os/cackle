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
        <Star className="h-6 w-6 text-[#880424]" />
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

  const images = [
    '/images/racing.jpeg',
    '/images/weezy.jpg',
    '/images/nicki.jpg'
  ];

  useEffect(() => {
    const fetchEventAndTickets = async () => {
      try {
        const [eventResult, ticketsResult] = await Promise.all([
          supabase
            .from('events')
            .select('*')
            .eq('id', id)
            .single(),
          supabase
            .from('ticket_types')
            .select('*')
            .eq('event_id', id)
        ]);

        if (eventResult.error) throw eventResult.error;
        if (ticketsResult.error) throw ticketsResult.error;
        
        if (typeof eventResult.data.policy_info === 'string') {
          try {
            eventResult.data.policy_info = JSON.parse(eventResult.data.policy_info);
          } catch (e) {
            console.error('Error parsing policy_info:', e);
          }
        }
        
        setEvent(eventResult.data);
        setTicketTypes(ticketsResult.data);
      } catch (err) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    };

    if (id) {
      fetchEventAndTickets();
    }
  }, [id]);

  useEffect(() => {
    const timer = setInterval(() => {
      setCurrentImage((prevImage) => (prevImage + 1) % images.length);
    }, 5000);
    return () => clearInterval(timer);
  }, [images.length]);

  if (loading) return (
    <div className="min-h-screen bg-gradient-to-br from-gray-900 via-[#880424] to-gray-900 flex items-center justify-center">
      <div className="text-white text-xl">Loading amazing events...</div>
    </div>
  );

  if (error) return (
    <div className="min-h-screen bg-gradient-to-br from-gray-900 via-[#880424] to-gray-900 flex items-center justify-center">
      <div className="text-red-500 text-xl">Error: {error}</div>
    </div>
  );

  if (!event) return (
    <div className="min-h-screen bg-gradient-to-br from-gray-900 via-[#880424] to-gray-900 flex items-center justify-center">
      <div className="text-white text-xl">Event not found</div>
    </div>
  );

  return (
    <>
      <Header className="fixed top-0 left-0 right-0 z-50" />
      <div className="flex flex-col min-h-screen bg-gradient-to-br from-gray-900 via-[#880424] to-gray-900 pt-16"> {/* Added pt-16 for header space */}
        <div className="relative group h-[60vh]">
          <ImageSlider images={images} currentImage={currentImage} />
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