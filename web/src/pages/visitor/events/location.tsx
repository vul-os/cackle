import React, { Suspense, lazy, useState } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { MapPin, ExternalLink } from 'lucide-react';

// ── Why this file does not draw a map by default ──────────────────────────
//
// The whole product claim is that Cackle works with no internet. This block
// used to mount a leaflet <TileLayer> pointed at
// `https://{s}.tile.openstreetmap.org/...` on every event page that had
// coordinates, which fires a burst of requests at a third party the moment
// the page renders. That is not a cosmetic inconsistency: it is the page
// telling a visitor "no network required" while contacting somebody else's
// server. And the organiser create-wizard exposes lat/lng, so a real event
// really does reach it — it was never dead code, only unreached by the demo
// seed, which is exactly how a defect like this survives a screenshot pass.
//
// So: FAIL CLOSED. No tile host is contacted unless the operator has named
// one they run themselves, via VITE_CACKLE_MAP_TILE_URL at build time. With
// it unset — the default, and what ships — this renders the address, the
// venue name and a link the visitor has to CHOOSE to click. Nothing leaves
// the origin on load.
//
// The leaflet bundle sits behind a dynamic import for the same reason: with
// no tile URL configured the map code is never fetched at all, so "no map"
// is a property of the build rather than a runtime branch.
const TILE_URL = import.meta.env.VITE_CACKLE_MAP_TILE_URL || '';
const TILE_ATTRIBUTION = import.meta.env.VITE_CACKLE_MAP_TILE_ATTRIBUTION || '';

const SelfHostedMap = lazy(() => import('./self-hosted-map'));

export interface LocationSectionProps {
    venueName?: string | null;
    address?: string | null;
    lat?: number | null;
    lng?: number | null;
}

/**
 * Venue/location block. The address plus an explicit "open in maps" link is
 * the source of truth and always renders with zero network dependency. An
 * embedded map appears only when the deployment configured a tile server it
 * controls.
 */
const LocationSection = ({ venueName, address, lat, lng }: LocationSectionProps) => {
    const [mapFailed, setMapFailed] = useState(false);
    const hasCoords = typeof lat === 'number' && typeof lng === 'number' && !(lat === 0 && lng === 0);
    const showMap = hasCoords && Boolean(TILE_URL) && !mapFailed;

    // A real <a>, not an onClick that calls window.open: it shows its
    // destination on hover, honours a middle-click, and — the point — is
    // only ever fetched because somebody deliberately clicked it.
    const directionsHref = hasCoords ? `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lng}#map=17/${lat}/${lng}` : null;

    return (
        <Card className="overflow-hidden">
            <CardContent className="p-5 sm:p-6">
                <div className="flex flex-wrap items-start justify-between gap-3">
                    <h2 className="font-display text-xl font-bold tracking-tight">Getting there</h2>
                    {directionsHref && (
                        <Button variant="outline" size="sm" asChild>
                            <a href={directionsHref} target="_blank" rel="noopener noreferrer">
                                <MapPin className="mr-2 h-4 w-4" aria-hidden="true" />
                                Open in maps
                                <ExternalLink className="ml-1.5 h-3.5 w-3.5 opacity-70" aria-hidden="true" />
                            </a>
                        </Button>
                    )}
                </div>

                <div className="mt-3 space-y-1">
                    {venueName && <p className="text-base font-semibold text-foreground">{venueName}</p>}
                    {address && <p className="text-sm leading-relaxed text-muted-foreground">{address}</p>}
                    {!venueName && !address && <p className="text-sm text-muted-foreground">Venue details to be announced.</p>}
                </div>

                {hasCoords && !TILE_URL && (
                    <p className="mt-4 border-t border-dashed border-border pt-3 text-xs leading-relaxed text-muted-foreground">
                        No map is drawn here. Cackle doesn&apos;t load map tiles from anybody else&apos;s server, so this page
                        keeps working with no internet. The link above opens a map in a new tab if you want one.
                    </p>
                )}
                {hasCoords && TILE_URL && mapFailed && (
                    <p className="mt-4 border-t border-dashed border-border pt-3 text-xs text-muted-foreground">
                        The configured map server didn&apos;t answer. The address above is what matters.
                    </p>
                )}
            </CardContent>

            {showMap && (
                <div className="aspect-video border-t border-border">
                    <Suspense fallback={<Skeleton className="h-full w-full rounded-none" />}>
                        <SelfHostedMap
                            lat={lat as number}
                            lng={lng as number}
                            label={venueName || address}
                            tileUrl={TILE_URL}
                            attribution={TILE_ATTRIBUTION}
                            onFailed={() => setMapFailed(true)}
                        />
                    </Suspense>
                </div>
            )}
        </Card>
    );
};

export default LocationSection;
