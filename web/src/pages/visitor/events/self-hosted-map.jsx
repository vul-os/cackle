import React from 'react';
import { MapContainer, TileLayer, Marker, Popup } from 'react-leaflet';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';

// Loaded ONLY via the dynamic import in ./location.jsx, and only when the
// deployment set VITE_CACKLE_MAP_TILE_URL to a tile server it runs itself.
// Keeping leaflet in its own chunk is what makes "Cackle contacts no third
// party" a build-time fact rather than a promise about a runtime branch: on
// a default build this module is never fetched, so there is no tile code in
// the page at all.
//
// `tileUrl` is whatever the operator configured. This file deliberately has
// no fallback URL — a default here would quietly reintroduce exactly the
// third-party fetch it exists to remove.

const markerIcon = L.divIcon({
    className: 'custom-div-icon',
    html: '<div style="background:hsl(var(--primary));width:28px;height:28px;border-radius:9999px;box-shadow:0 4px 12px rgba(0,0,0,0.35);border:3px solid white;"></div>',
    iconSize: [28, 28],
    iconAnchor: [14, 14],
    popupAnchor: [0, -14],
});

export default function SelfHostedMap({ lat, lng, label, tileUrl, attribution, onFailed }) {
    if (!tileUrl) return null;
    return (
        <MapContainer center={[lat, lng]} zoom={15} className="h-full w-full" zoomControl={false} scrollWheelZoom={false}>
            <TileLayer
                url={tileUrl}
                attribution={attribution || undefined}
                eventHandlers={{ tileerror: () => onFailed?.() }}
            />
            <Marker position={[lat, lng]} icon={markerIcon}>
                <Popup>{label}</Popup>
            </Marker>
        </MapContainer>
    );
}
