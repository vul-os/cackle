import React, { useState } from 'react';
import { MapContainer, TileLayer, Marker, Popup } from 'react-leaflet';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { MapPin } from 'lucide-react';
import { useOnline } from '@/lib/use-online';

const customIcon = L.divIcon({
    className: 'custom-div-icon',
    html: `<div style="background:hsl(var(--primary));width:28px;height:28px;border-radius:9999px;box-shadow:0 4px 12px rgba(0,0,0,0.35);border:3px solid white;"></div>`,
    iconSize: [28, 28],
    iconAnchor: [14, 14],
    popupAnchor: [0, -14],
});

/**
 * Venue/location block. The address + "Get directions" text is the source
 * of truth and always renders with no network dependency. The embedded map
 * is a bonus visual on top of that — it needs a remote tile host, so it's
 * only attempted while the browser reports itself online, and it quietly
 * withdraws (falling back to the static block) if enough tiles fail to
 * load, rather than leaving a half-broken grey grid on screen.
 */
const LocationSection = ({ venueName, address, lat, lng }) => {
    const online = useOnline();
    const [mapFailed, setMapFailed] = useState(false);
    const hasCoords = typeof lat === 'number' && typeof lng === 'number' && !(lat === 0 && lng === 0);
    const showMap = hasCoords && online && !mapFailed;

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
                {hasCoords && !online && (
                    <p className="mt-2 text-xs text-muted-foreground">Map preview needs a connection — you&apos;re offline right now.</p>
                )}
            </CardContent>
            {showMap && (
                <div className="aspect-video">
                    <MapContainer center={[lat, lng]} zoom={15} className="h-full w-full" zoomControl={false} scrollWheelZoom={false}>
                        <TileLayer
                            attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
                            url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
                            eventHandlers={{
                                tileerror: () => setMapFailed(true),
                            }}
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
