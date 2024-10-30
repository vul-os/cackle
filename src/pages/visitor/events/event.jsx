import React from 'react';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Calendar, MapPin, Info, Ticket, Clock } from 'lucide-react';

const EventPage = () => {
  const eventData = {
    title: "Summer Music Festival 2024",
    date: "Aug 15 - Aug 18 2024",
    time: "11:00 AM - 11:00 PM",
    location: "Riverside Park, Green Valley",
    description: "4 days, 4 stages, 50+ live bands! Join us for the biggest music festival of the year. Featuring top artists from around the world, amazing food vendors, and unforgettable experiences.",
    category: "Festival",
    ageRequirement: "18+",
    parking: "Free parking available on site",
    prohibitedItems: "No outside food or drinks, No weapons, No pets",
    organizer: "Valley Events Ltd."
  };

  return (
    <div className="flex flex-col min-h-screen">
      {/* Hero Section */}
      <div className="relative h-96 bg-gradient-to-b from-purple-900 to-purple-800">
        <img
          src="/api/placeholder/1200/400"
          alt="Event cover"
          className="w-full h-full object-cover opacity-50"
        />
        <div className="absolute inset-0 bg-black bg-opacity-40" />
        <div className="absolute bottom-0 left-0 right-0 p-8">
          <div className="max-w-4xl mx-auto">
            <span className="inline-block bg-white bg-opacity-20 text-white px-3 py-1 rounded-full text-sm mb-4">
              {eventData.category}
            </span>
            <h1 className="text-4xl md:text-5xl font-bold text-white mb-4">
              {eventData.title}
            </h1>
          </div>
        </div>
      </div>

      {/* Event Info Bar */}
      <div className="bg-white border-b">
        <div className="max-w-4xl mx-auto p-4">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="flex items-center gap-3">
              <Calendar className="h-5 w-5 text-gray-500" />
              <div>
                <p className="text-sm font-medium">{eventData.date}</p>
                <p className="text-sm text-gray-500">{eventData.time}</p>
              </div>
            </div>
            <div className="flex items-center gap-3">
              <MapPin className="h-5 w-5 text-gray-500" />
              <div>
                <p className="text-sm font-medium">{eventData.location}</p>
                <p className="text-sm text-gray-500">View on map</p>
              </div>
            </div>
            <div className="flex items-center justify-end">
              <Button className="bg-purple-600 hover:bg-purple-700">
                <Ticket className="h-4 w-4 mr-2" />
                Buy Tickets
              </Button>
            </div>
          </div>
        </div>
      </div>

      {/* Main Content */}
      <div className="max-w-4xl mx-auto p-4 grid grid-cols-1 md:grid-cols-3 gap-8 my-8">
        {/* Left Column */}
        <div className="col-span-2">
          <Card>
            <CardHeader>
              <CardTitle>Event Details</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-gray-600">
                {eventData.description}
              </p>
            </CardContent>
          </Card>

          <Card className="mt-8">
            <CardHeader>
              <CardTitle>Location</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="aspect-video bg-gray-100 rounded-lg">
                <img
                  src="/api/placeholder/800/400"
                  alt="Event location map"
                  className="w-full h-full object-cover rounded-lg"
                />
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Right Column */}
        <div className="col-span-1">
          <Card>
            <CardHeader>
              <CardTitle>Information</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div>
                <h3 className="text-sm font-medium text-gray-500">Age Requirement</h3>
                <p className="mt-1">{eventData.ageRequirement}</p>
              </div>
              <div>
                <h3 className="text-sm font-medium text-gray-500">Parking</h3>
                <p className="mt-1">{eventData.parking}</p>
              </div>
              <div>
                <h3 className="text-sm font-medium text-gray-500">Prohibited Items</h3>
                <p className="mt-1">{eventData.prohibitedItems}</p>
              </div>
            </CardContent>
          </Card>

          <Card className="mt-8">
            <CardHeader>
              <CardTitle>Organizer</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex items-center gap-4">
                <div className="h-12 w-12 bg-gray-100 rounded-full">
                  <img
                    src="/api/placeholder/48/48"
                    alt="Organizer logo"
                    className="w-full h-full rounded-full"
                  />
                </div>
                <div>
                  <h3 className="font-medium">{eventData.organizer}</h3>
                  <p className="text-sm text-gray-500">Event Organizer</p>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
};

export default EventPage;