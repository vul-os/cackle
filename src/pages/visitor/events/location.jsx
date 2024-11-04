import React from 'react';
import { MapContainer, TileLayer, Marker, Popup } from 'react-leaflet';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import {
  Card,
  CardContent,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { MapPin } from 'lucide-react';

// Map marker configuration
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

  export default LocationSection