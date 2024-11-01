import React, { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { MapContainer, TileLayer, Marker, Popup } from 'react-leaflet';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import { supabase } from '@/services/supabaseClient';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Calendar, MapPin, Info, Ticket, Clock, ArrowRight } from 'lucide-react';

// Custom marker icon
const customIcon = L.divIcon({
  className: 'custom-div-icon',
  html: `
    <div style="
      background-color: #7766F7;
      width: 30px;
      height: 30px;
      border-radius: 50%;
      position: relative;
      box-shadow: 0 2px 5px rgba(0,0,0,0.2);
    ">
      <div style="
        position: absolute;
        bottom: -8px;
        left: 50%;
        transform: translateX(-50%);
        width: 0;
        height: 0;
        border-left: 8px solid transparent;
        border-right: 8px solid transparent;
        border-top: 8px solid #7766F7;
      "></div>
    </div>
  `,
  iconSize: [30, 42],
  iconAnchor: [15, 42],
  popupAnchor: [0, -42],
});

const ImageSlider = ({ images, currentImage }) => (
  <div className="h-screen max-h-[500px] relative overflow-hidden">
    {images.map((image, index) => (
      <div
        key={index}
        className={`absolute inset-0 transition-opacity duration-1000 ${
          index === currentImage ? 'opacity-100' : 'opacity-0'
        }`}
        style={{
          backgroundImage: `url(${image})`,
          backgroundSize: 'cover',
          backgroundPosition: 'center',
          backgroundRepeat: 'no-repeat',
          backgroundColor: '#000'
        }}
      />
    ))}
    <div className="absolute inset-0 bg-[#7766F7]/30" />
  </div>
);

const EventHeader = ({ category, title }) => (
  <div className="absolute bottom-0 left-0 right-0 p-12 bg-gradient-to-t from-black/80 to-transparent">
    <div className="max-w-4xl mx-auto">
      <span className="inline-block bg-[#7766F7] text-white px-4 py-1.5 rounded-full text-sm font-medium mb-6 backdrop-blur-sm">
        {category}
      </span>
      <h1 className="text-5xl md:text-6xl font-bold text-white mb-6 drop-shadow-lg">
        {title}
      </h1>
    </div>
  </div>
);

const EventQuickInfo = ({ date, time, location }) => (
  <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
    <QuickInfoItem 
      icon={<Calendar className="h-6 w-6 text-[#7766F7]" />} 
      title={new Date(date).toLocaleDateString('en-US', { 
        month: 'long',
        day: 'numeric',
        year: 'numeric'
      })} 
      subtitle={time} 
    />
    <QuickInfoItem 
      icon={<MapPin className="h-6 w-6 text-[#7766F7]" />} 
      title={location} 
      subtitle={<span className="text-[#7766F7] group-hover:underline">View on map</span>} 
    />
    <div className="flex items-center justify-end">
      <Button className="bg-[#7766F7] hover:bg-[#7766F7]/90 text-lg px-6 py-6 shadow-lg hover:shadow-xl transition-all text-white">
        <Ticket className="h-5 w-5 mr-2" />
        Buy Tickets
      </Button>
    </div>
  </div>
);

const QuickInfoItem = ({ icon, title, subtitle }) => (
  <div className="flex items-center gap-4 group cursor-pointer">
    <div className="p-3 rounded-full bg-[#7766F7]/10 group-hover:bg-[#7766F7]/20 transition-colors">
      {icon}
    </div>
    <div>
      <p className="font-semibold text-gray-100">{title}</p>
      <p className="text-gray-400">{subtitle}</p>
    </div>
  </div>
);

const LocationSection = ({ location, latitude, longitude }) => (
  <div className="rounded-2xl overflow-hidden bg-gray-900 shadow-lg border border-gray-800">
    <div className="aspect-video">
      <MapContainer 
        center={[latitude, longitude]} 
        zoom={15} 
        style={{ height: '100%', width: '100%' }}
      >
        <TileLayer
          url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
          attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
        />
        <Marker position={[latitude, longitude]} icon={customIcon}>
          <Popup>
            <div className="p-2">
              <h3 className="font-semibold">{location}</h3>
            </div>
          </Popup>
        </Marker>
      </MapContainer>
    </div>
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-xl font-semibold text-[#7766F7]">Event Location</h3>
        <Button 
          variant="outline" 
          className="text-[#7766F7] border-[#7766F7] hover:bg-[#7766F7]/10"
          onClick={() => window.open(`https://www.google.com/maps/search/?api=1&query=${latitude},${longitude}`)}
        >
          <MapPin className="h-4 w-4 mr-2" />
          Get Directions
        </Button>
      </div>
      <p className="text-gray-300">{location}</p>
    </div>
  </div>
);

const EventDetailsSection = ({ description }) => (
  <div className="bg-gray-900 rounded-2xl shadow-lg overflow-hidden border border-gray-800">
    <div className="p-6">
      <h2 className="text-2xl font-semibold text-[#7766F7] mb-4">Event Details</h2>
      <div className="prose prose-lg prose-invert">
        <p className="text-gray-300 leading-relaxed">{description}</p>
      </div>
    </div>
  </div>
);

const InformationSection = ({ policyInfo }) => {
  return (
    <div className="bg-gray-900 rounded-2xl shadow-lg border border-gray-800">
      <div className="p-6">
        <h2 className="text-2xl font-semibold text-[#7766F7] mb-6">Information</h2>
        <div className="space-y-8">
          {policyInfo && Object.entries(policyInfo).map(([category, items]) => (
            <div key={category} className="border-b border-gray-800 last:border-0 pb-6 last:pb-0">
              <h3 className="text-lg font-medium mb-4 text-gray-100">{category}</h3>
              <ul className="space-y-3">
                {Array.isArray(items) && items.map((item, index) => (
                  <li key={index} className="flex items-start gap-3">
                    <div className="mt-1">
                      <Info className="h-4 w-4 text-[#7766F7]" />
                    </div>
                    <span className="text-gray-300">{item}</span>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};

const EventPage = () => {
  const { id } = useParams();
  const [event, setEvent] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [currentImage, setCurrentImage] = useState(0);

  // Placeholder images - replace with actual event images
  const images = [
    '/api/placeholder/1200/600',
    '/api/placeholder/1200/600',
    '/api/placeholder/1200/600'
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
  }, []);

  if (loading) return <div className="min-h-screen bg-black flex items-center justify-center">
    <div className="text-white">Loading...</div>
  </div>;

  if (error) return <div className="min-h-screen bg-black flex items-center justify-center">
    <div className="text-red-500">Error: {error}</div>
  </div>;

  if (!event) return <div className="min-h-screen bg-black flex items-center justify-center">
    <div className="text-white">Event not found</div>
  </div>;

  const getTimeString = (timestamp) => {
    return new Date(timestamp).toLocaleTimeString('en-US', {
      hour: 'numeric',
      minute: '2-digit',
      hour12: true
    });
  };

  return (
    <div className="flex flex-col min-h-screen bg-black">
      <div className="relative">
        <ImageSlider images={images} currentImage={currentImage} />
        <EventHeader category={event.category} title={event.title} />
      </div>

      <div className="bg-gray-900/50 backdrop-blur shadow-lg border-t border-gray-800">
        <div className="max-w-4xl mx-auto p-6">
          <EventQuickInfo 
            date={event.start_time}
            time={`${getTimeString(event.start_time)} - ${getTimeString(event.end_time)}`}
            location={event.venue_name}
          />
        </div>
      </div>

      <div className="max-w-4xl mx-auto p-4 grid grid-cols-1 md:grid-cols-3 gap-8 my-8">
        <div className="col-span-2 space-y-8">
          <EventDetailsSection description={event.description} />
          <LocationSection 
            location={event.venue_address}
            latitude={event.venue_latitude}
            longitude={event.venue_longitude}
          />
        </div>
        <div className="col-span-1">
          <InformationSection policyInfo={event.policy_info} />
        </div>
      </div>
    </div>
  );
};

export default EventPage;