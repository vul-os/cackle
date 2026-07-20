import React from 'react';
import { MapContainer, TileLayer, Marker, Popup } from 'react-leaflet';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { MapPin } from 'lucide-react';

const customIcon = L.divIcon({
    className: 'custom-div-icon',
    html: `<div style="background:hsl(var(--primary));width:28px;height:28px;border-radius:9999px;box-shadow:0 4px 12px rgba(0,0,0,0.35);border:3px solid white;"></div>`,
    iconSize: [28, 28],
    iconAnchor: [14, 14],
    popupAnchor: [0, -14],
});

const LocationSection = ({ venueName, address, lat, lng }) => {
    const hasCoords = typeof lat === 'number' && typeof lng === 'number' && !(lat === 0 && lng === 0);

    return (
        <Card className="overflow-hidden">
            <CardContent className="p-6">
                <div className="flex items-center justify-between">
                    <h3 className="font-display text-xl font-bold">Location</h3>
                    {hasCoords && (
                        <Button
                            variant="outline"
                            size="sm"
                            onClick={() => window.open(`https://www.google.com/maps/search/?api=1&query=${lat},${lng}`, '_blank')}
                        >
                            <MapPin className="mr-2 h-4 w-4" />
                            Get Directions
                        </Button>
                    )}
                </div>
                {venueName && <p className="mt-2 font-medium">{venueName}</p>}
                {address && <p className="text-sm text-muted-foreground">{address}</p>}
            </CardContent>
            {hasCoords && (
                <div className="aspect-video">
                    <MapContainer center={[lat, lng]} zoom={15} className="h-full w-full" zoomControl={false} scrollWheelZoom={false}>
                        <TileLayer
                            attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
                            url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
                        />
                        <Marker position={[lat, lng]} icon={customIcon}>
                            <Popup>{venueName || address}</Popup>
                        </Marker>
                    </MapContainer>
                </div>
            )}
        </Card>
    );
};

export default LocationSection;
