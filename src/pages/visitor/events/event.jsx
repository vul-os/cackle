import React, { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { MapContainer, TileLayer, Marker, Popup } from 'react-leaflet';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import { supabase } from '@/services/supabaseClient';
import {
  Card,
  CardContent,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Calendar, MapPin, Info, Ticket, Share2, Heart, Star } from 'lucide-react';

// Custom text processing component for event details
const ProcessedText = ({ content, className = "" }) => {
  const processContent = (text) => {
    if (!text) return "";

    // Process the text step by step
    let processed = text
      // Replace section headers (##) with styled headers
      .replace(/##\s?(.*?)\\n/g, '<h2 class="text-xl font-bold text-white mt-6 mb-3">$1</h2>')
      // Replace subsection headers (#) with styled headers
      .replace(/#\s?(.*?)\\n/g, '<h3 class="text-lg font-semibold text-white mt-4 mb-2">$1</h3>')
      // Handle bullet points
      .replace(/\\n-\s?(.*?)(?=\\n|$)/g, '<li class="flex items-center gap-2"><span class="w-1.5 h-1.5 bg-white/50 rounded-full"></span>$1</li>')
      // Convert checkmarks
      .replace(/✅/g, '<span class="text-green-500">✓</span>')
      // Handle line breaks
      .split('\\n\\n').join('</p><p class="mt-4">')
      .split('\\n').join('<br>')
      // Wrap in paragraph if not already wrapped
      .replace(/^(?!<[hp])/g, '<p>');

    return processed;
  };

  return (
    <div 
      className={`text-gray-200 leading-relaxed ${className}`}
      dangerouslySetInnerHTML={{ __html: processContent(content) }}
    />
  );
};

const ImageSlider = ({ images, currentImage }) => (
  <div className="relative w-full h-full">
    {images.map((image, index) => (
      <div
        key={index}
        className={`absolute inset-0 transition-transform duration-1000 ${
          index === currentImage ? 'scale-100 opacity-100' : 'scale-110 opacity-0'
        }`}
      >
        <img 
          src={image} 
          alt={`Slide ${index + 1}`}
          className="w-full h-full object-contain"
        />
        <div className="absolute inset-0 bg-gradient-to-b from-black/30 via-transparent to-black/80" />
      </div>
    ))}
  </div>
);

const customIcon = L.divIcon({
  className: 'custom-div-icon',
  html: `
    <div style="
      background-color: #880424;
      width: 36px;
      height: 36px;
      border-radius: 50%;
      position: relative;
      box-shadow: 0 4px 12px rgba(136,4,36,0.3);
      display: flex;
      align-items: center;
      justify-content: center;
      animation: pulse 2s infinite;
    ">
      <div style="
        position: absolute;
        bottom: -10px;
        left: 50%;
        transform: translateX(-50%);
        width: 0;
        height: 0;
        border-left: 10px solid transparent;
        border-right: 10px solid transparent;
        border-top: 10px solid #880424;
      "></div>
    </div>
  `,
  iconSize: [36, 48],
  iconAnchor: [18, 48],
  popupAnchor: [0, -48],
});

const EventHeader = ({ category, title }) => (
  <div className="absolute bottom-0 left-0 right-0 p-12">
    <div className="max-w-5xl mx-auto">
      <div className="space-y-6 transform translate-y-8 transition-transform duration-300 group-hover:translate-y-0">
        <span className="inline-block bg-gradient-to-r from-[#880424] to-[#660318] text-white px-6 py-2 rounded-full text-sm font-medium backdrop-blur-sm animate-bounce">
          {category}
        </span>
        <h1 className="text-6xl md:text-7xl font-bold text-white mb-6 drop-shadow-lg">
          {title}
        </h1>
        <div className="flex gap-4">
          <Button variant="outline" className="bg-white/10 backdrop-blur-md border-none text-white hover:bg-white/20">
            <Heart className="h-5 w-5 mr-2" />
            Save Event
          </Button>
          <Button variant="outline" className="bg-white/10 backdrop-blur-md border-none text-white hover:bg-white/20">
            <Share2 className="h-5 w-5 mr-2" />
            Share
          </Button>
        </div>
      </div>
    </div>
  </div>
);

const QuickInfoItem = ({ icon, title, subtitle }) => (
  <div className="flex items-center gap-4 group cursor-pointer transform hover:scale-105 transition-transform duration-300">
    <div className="p-3 rounded-full bg-gradient-to-br from-[#880424]/10 to-[#660318]/10 group-hover:from-[#880424]/20 group-hover:to-[#660318]/20 transition-colors">
      {icon}
    </div>
    <div>
      <p className="font-semibold text-white">{title}</p>
      <p className="text-gray-200">{subtitle}</p>
    </div>
  </div>
);

const EventQuickInfo = ({ date, time, location }) => (
  <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
    <QuickInfoItem 
      icon={<Calendar className="h-6 w-6 text-[#880424]" />} 
      title={date ? new Date(date).toLocaleDateString('en-US', { 
        month: 'long',
        day: 'numeric',
        year: 'numeric'
      }) : 'Date TBA'} 
      subtitle={time} 
    />
    <QuickInfoItem 
      icon={<MapPin className="h-6 w-6 text-[#880424]" />} 
      title={location} 
      subtitle={<span className="text-[#880424] group-hover:underline">View on map</span>} 
    />
    <div className="flex items-center justify-end">
      <Button className="bg-gradient-to-r from-[#880424] to-[#660318] hover:from-[#990525] hover:to-[#770419] text-lg px-8 py-6 shadow-lg hover:shadow-xl transition-all text-white rounded-full animate-pulse">
        <Ticket className="h-5 w-5 mr-2" />
        Get Tickets
      </Button>
    </div>
  </div>
);

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

const LocationSection = ({ location, latitude, longitude }) => {
  const handleGetDirections = () => {
    window.open(`https://www.google.com/maps/search/?api=1&query=${latitude},${longitude}`);
  };

  return (
    <Card className="overflow-hidden border-none bg-white/5 backdrop-blur-lg hover:bg-white/10 transition-colors duration-300">
      <div className="aspect-video relative">
        {typeof window !== 'undefined' && (
          <MapContainer 
            center={[latitude || 0, longitude || 0]} 
            zoom={15} 
            className="h-full w-full"
            zoomControl={false}
          >
            <TileLayer
              attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
              url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
            />
            <Marker position={[latitude || 0, longitude || 0]} icon={customIcon}>
              <Popup>
                <div className="p-2">
                  <h3 className="font-semibold">{location}</h3>
                </div>
              </Popup>
            </Marker>
          </MapContainer>
        )}
      </div>
      <CardContent className="p-8">
        <div className="flex items-center justify-between">
          <h3 className="text-2xl font-bold text-white">Event Location</h3>
          <Button 
            variant="outline" 
            className="border-[#880424] text-[#880424] hover:bg-[#880424] hover:text-white transition-colors duration-300"
            onClick={handleGetDirections}
          >
            <MapPin className="h-4 w-4 mr-2" />
            Get Directions
          </Button>
        </div>
        <p className="text-gray-100 mt-4">{location}</p>
      </CardContent>
    </Card>
  );
};

const InformationSection = ({ information, policyInfo }) => (
  <Card className="border-none bg-white/5 backdrop-blur-lg hover:bg-white/10 transition-colors duration-300">
    <CardContent className="p-8">
      <h2 className="text-2xl font-bold text-white mb-6">Information</h2>
      <ProcessedText content={information} className="mb-8" />
      
      {policyInfo && typeof policyInfo === 'string' && (
        <ProcessedText content={policyInfo} />
      )}
      
      {policyInfo && typeof policyInfo === 'object' && (
        <div className="space-y-6">
          {Object.entries(policyInfo).map(([category, items]) => (
            <div key={category} className="border-b border-white/10 last:border-0 pb-6 last:pb-0">
              <h3 className="text-lg font-medium mb-4 text-white">{category}</h3>
              <ul className="space-y-3">
                {Array.isArray(items) && items.map((item, index) => (
                  <li key={index} className="flex items-start gap-3">
                    <Info className="h-4 w-4 text-[#880424] mt-1 flex-shrink-0" />
                    <span className="text-gray-100">{item}</span>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </CardContent>
  </Card>
);

const EventPage = () => {
  const { id } = useParams();
  const [event, setEvent] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [currentImage, setCurrentImage] = useState(0);

  const images = [
    '/images/racing.jpeg',
    '/images/weezy.jpg',
    '/images/nicki.jpg'
  ];

  useEffect(() => {
    const fetchEvent = async () => {
      try {
        const { data, error } = await supabase
          .from('events')
          .select('*')
          .eq('id', id)
          .single();

        if (error) throw error;
        
        if (typeof data.policy_info === 'string') {
          try {
            data.policy_info = JSON.parse(data.policy_info);
          } catch (e) {
            console.error('Error parsing policy_info:', e);
          }
        }
        
        setEvent(data);
      } catch (err) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    };

    if (id) {
      fetchEvent();
    }
  }, [id]);

  useEffect(() => {
    const timer = setInterval(() => {
      setCurrentImage((prevImage) => (prevImage + 1) % images.length);
    }, 5000);
    return () => clearInterval(timer);
  }, [images.length]);

  const getTimeString = (timestamp) => {
    if (!timestamp) return '';
    return new Date(timestamp).toLocaleTimeString('en-US', {
      hour: 'numeric',
      minute: '2-digit',
      hour12: true
    });
  };

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
    <div className="flex flex-col min-h-screen bg-gradient-to-br from-gray-900 via-[#880424] to-gray-900">
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
            date={event.start_time}
            time={`${getTimeString(event.start_time)} - ${getTimeString(event.end_time)}`}
            location={event.venue_name || 'Venue'}
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
  );
};

export default EventPage;