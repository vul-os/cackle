import React from 'react';
import { MapContainer, TileLayer, Marker, Popup } from 'react-leaflet';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';

// Loaded ONLY via the dynamic import in ./location.tsx, and only when the
// deployment set VITE_CACKLE_MAP_TILE_URL to a tile server it runs itself.
// Keeping leaflet in its own chunk is what makes "Cackle contacts no third
// party" a build-time fact rather than a promise about a runtime branch: on
// a default build this module is never fetched, so there is no tile code in
// the page at all.
//
// `tileUrl` is whatever the operator configured. This file deliberately has
// no fallback URL — a default here would quietly reintroduce exactly the
// third-party fetch it exists to remove.

// Leaflet's divIcon takes a raw HTML string, not JSX, so this can't reach
// Tailwind's token classes — but every colour below is still a CSS custom
// property reference (`hsl(var(--x))`), never a literal, so it reads exactly
// like a token to anything auditing this file. The fill is `--primary`, on
// brand. The ring and drop-shadow use `--on-media` / `--on-media-ground` —
// the same tokens the photo-overlay chrome in ./gallery.tsx and
// ./header.tsx uses — because this pin sits on top of an arbitrary map TILE
// image, never the app's own surface, and needs to read against any tile
// style regardless of the OS theme. Only reached at all when an operator has
// configured VITE_CACKLE_MAP_TILE_URL.
const markerIcon = L.divIcon({
    className: 'custom-div-icon',
    html: '<div style="background:hsl(var(--primary));width:28px;height:28px;border-radius:9999px;box-shadow:0 4px 12px hsl(var(--on-media-ground)/0.35);border:3px solid hsl(var(--on-media));"></div>',
    iconSize: [28, 28],
    iconAnchor: [14, 14],
    popupAnchor: [0, -14],
});

export interface SelfHostedMapProps {
    lat: number;
    lng: number;
    label?: string | null;
    tileUrl: string;
    attribution?: string;
    onFailed?: () => void;
}

export default function SelfHostedMap({ lat, lng, label, tileUrl, attribution, onFailed }: SelfHostedMapProps) {
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
