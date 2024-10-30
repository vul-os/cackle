import React, { useState, useEffect } from 'react';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Calendar, MapPin, Info, Ticket, Clock, ArrowRight } from 'lucide-react';

const EventMap = () => (
  <svg viewBox="0 0 800 400" className="w-full h-full">
    <rect width="800" height="400" fill="#111827"/>
    <path d="M0 200 H800" stroke="#1f2937" strokeWidth="20"/>
    <path d="M400 0 V400" stroke="#1f2937" strokeWidth="15"/>
    <path d="M100 50 H700 V350 H100 Z" fill="#1f2937" stroke="#7766F7" strokeWidth="2"/>
    <rect x="150" y="100" width="500" height="200" fill="#7766F7" fillOpacity="0.1" stroke="#7766F7" strokeWidth="2"/>
    <g fill="#7766F7" fillOpacity="0.2">
      <rect x="200" y="120" width="60" height="40"/>
      <rect x="540" y="120" width="60" height="40"/>
      <rect x="200" y="240" width="60" height="40"/>
      <rect x="540" y="240" width="60" height="40"/>
    </g>
    <g fill="#374151">
      <rect x="150" y="320" width="100" height="20"/>
      <rect x="550" y="320" width="100" height="20"/>
    </g>
    <g fontSize="12" fill="#e5e7eb" textAnchor="middle">
      <text x="400" y="30">RIVERSIDE PARK</text>
      <text x="230" y="140">MAIN STAGE</text>
      <text x="570" y="140">ELECTRONIC</text>
      <text x="230" y="260">ROCK STAGE</text>
      <text x="570" y="260">INDIE STAGE</text>
      <text x="200" y="350">PARKING</text>
      <text x="600" y="350">PARKING</text>
    </g>
    <g fontSize="10" fill="#9ca3af">
      <text x="410" y="190">River Road</text>
      <text x="380" y="210" transform="rotate(90 380 210)">Park Avenue</text>
    </g>
  </svg>
);

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
          backgroundSize: 'contain',
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
    <QuickInfoItem icon={<Calendar className="h-6 w-6 text-[#7766F7]" />} 
      title={date} subtitle={time} />
    <QuickInfoItem icon={<MapPin className="h-6 w-6 text-[#7766F7]" />} 
      title={location} subtitle={<span className="text-[#7766F7] group-hover:underline">View on map</span>} />
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

const LocationSection = ({ location }) => (
  <div className="rounded-2xl overflow-hidden bg-gray-900 shadow-lg border border-gray-800">
    <div className="aspect-video">
      <EventMap />
    </div>
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-xl font-semibold text-[#7766F7]">Event Location</h3>
        <Button variant="outline" className="text-[#7766F7] border-[#7766F7] hover:bg-[#7766F7]/10">
          <MapPin className="h-4 w-4 mr-2" />
          Get Directions
        </Button>
      </div>
      <p className="text-gray-300">{location}</p>
      <div className="grid grid-cols-2 gap-4 mt-4">
        <div className="bg-gray-800 p-4 rounded-lg border border-gray-700">
          <h4 className="font-medium mb-2 text-gray-100">Parking Areas</h4>
          <p className="text-sm text-gray-400">Available at North and South entrance</p>
        </div>
        <div className="bg-gray-800 p-4 rounded-lg border border-gray-700">
          <h4 className="font-medium mb-2 text-gray-100">Entry Gates</h4>
          <p className="text-sm text-gray-400">4 gates around the venue</p>
        </div>
      </div>
    </div>
  </div>
);

const EventDetailsSection = ({ description }) => (
  <div className="bg-gray-900 rounded-2xl shadow-lg overflow-hidden border border-gray-800">
    <div className="p-6">
      <h2 className="text-2xl font-semibold text-[#7766F7] mb-4">Event Details</h2>
      <div className="prose prose-lg prose-invert">
        <p className="text-gray-300 leading-relaxed">{description}</p>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mt-8">
          <div className="bg-gray-800 p-6 rounded-xl border border-gray-700">
            <h3 className="font-medium text-lg mb-4 text-gray-100">Schedule Highlights</h3>
            <ul className="space-y-3">
              <li className="flex items-center gap-2 text-gray-300">
                <Clock className="h-4 w-4 text-[#7766F7]" />
                <span>Day 1: Opening Ceremony</span>
              </li>
              <li className="flex items-center gap-2 text-gray-300">
                <Clock className="h-4 w-4 text-[#7766F7]" />
                <span>Day 2: Main Performances</span>
              </li>
              <li className="flex items-center gap-2 text-gray-300">
                <Clock className="h-4 w-4 text-[#7766F7]" />
                <span>Day 3: Special Events</span>
              </li>
            </ul>
          </div>
          <div className="bg-gray-800 p-6 rounded-xl border border-gray-700">
            <h3 className="font-medium text-lg mb-4 text-gray-100">Amenities</h3>
            <ul className="space-y-3">
              <li className="flex items-center gap-2 text-gray-300">
                <Info className="h-4 w-4 text-[#7766F7]" />
                <span>Food & Beverage Stations</span>
              </li>
              <li className="flex items-center gap-2 text-gray-300">
                <Info className="h-4 w-4 text-[#7766F7]" />
                <span>First Aid Stations</span>
              </li>
              <li className="flex items-center gap-2 text-gray-300">
                <Info className="h-4 w-4 text-[#7766F7]" />
                <span>Rest Areas</span>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  </div>
);

const InformationSection = () => {
  const requirements = {
    "Event Rules": [
      "18+ only",
      "No outside food or drinks", 
      "No pets allowed"
    ],
    "What to Bring": [
      "Valid ID",
      "Comfortable shoes",
      "Cash for vendors"
    ],
    "Safety": [
      "First aid stations available",
      "Security checkpoints at all entrances",
      "Emergency exits clearly marked"
    ]
  };

  return (
    <div className="bg-gray-900 rounded-2xl shadow-lg border border-gray-800">
      <div className="p-6">
        <h2 className="text-2xl font-semibold text-[#7766F7] mb-6">Information</h2>
        <div className="space-y-8">
          {Object.entries(requirements).map(([category, items]) => (
            <div key={category} className="border-b border-gray-800 last:border-0 pb-6 last:pb-0">
              <h3 className="text-lg font-medium mb-4 text-gray-100">{category}</h3>
              <ul className="space-y-3">
                {items.map((item, index) => (
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
  const [currentImage, setCurrentImage] = useState(0);
  const images = [
    '/images/event.jpeg',
    '/images/music.jpg',
    '/images/ranch.jpg',
    '/images/coffee.jpg'
  ];

  const eventData = {
    title: "Summer Music Festival 2024",
    date: "Aug 15 - Aug 18 2024",
    time: "11:00 AM - 11:00 PM",
    location: "Riverside Park, Green Valley",
    description: "4 days, 4 stages, 50+ live bands! Join us for the biggest music festival of the year. Featuring top artists from around the world, amazing food vendors, and unforgettable experiences.",
    category: "Festival",
    organizer: "Valley Events Ltd."
  };

  useEffect(() => {
    const timer = setInterval(() => {
      setCurrentImage((prevImage) => (prevImage + 1) % images.length);
    }, 5000);
    return () => clearInterval(timer);
  }, []);

  return (
    <div className="flex flex-col min-h-screen bg-black">
      <div className="relative">
        <ImageSlider images={images} currentImage={currentImage} />
        <EventHeader category={eventData.category} title={eventData.title} />
      </div>

      <div className="bg-gray-900/50 backdrop-blur shadow-lg border-t border-gray-800">
        <div className="max-w-4xl mx-auto p-6">
          <EventQuickInfo 
            date={eventData.date} 
            time={eventData.time} 
            location={eventData.location} 
          />
        </div>
      </div>

      <div className="max-w-4xl mx-auto p-4 grid grid-cols-1 md:grid-cols-3 gap-8 my-8">
        <div className="col-span-2 space-y-8">
          <EventDetailsSection description={eventData.description} />
          <LocationSection location={eventData.location} />
        </div>
        <div className="col-span-1">
          <InformationSection />
        </div>
      </div>
    </div>
  );
};

export default EventPage;