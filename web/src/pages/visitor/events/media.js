// Shared helpers for resolving an event's cover image + gallery into plain
// { id, url, alt } descriptors, regardless of which shape the backend hands
// back. Event media has gone through a couple of shapes in this codebase
// (a bare `cover_image` URL string from the original seed data; the newer
// `cover_image_id` + `gallery` join backed by `POST /api/events/{id}/images`
// and served from `GET /media/{id}`) — resolving defensively here means the
// storefront never blanks out just because one shape didn't come back.
import { images as imagesApi } from '@/lib/api';

/** Turns a single image reference (string URL, or {id,url,...}) into a URL. */
export function resolveImageUrl(img) {
    if (!img) return null;
    if (typeof img === 'string') return img.trim() || null;
    if (img.url) return img.url;
    if (img.id) return imagesApi.url(img.id);
    return null;
}

/**
 * Builds the ordered list of images to show for an event: the cover image
 * first (if any), followed by any additional gallery images, de-duplicated
 * by resolved URL. Always returns an array (possibly empty) — never throws.
 */
export function getEventImages(event) {
    if (!event) return [];
    const seen = new Set();
    const images = [];

    const push = (ref, altFallback) => {
        const url = resolveImageUrl(ref);
        if (!url || seen.has(url)) return;
        seen.add(url);
        images.push({
            id: (ref && typeof ref === 'object' && ref.id) || url,
            url,
            width: ref && typeof ref === 'object' ? ref.width : undefined,
            height: ref && typeof ref === 'object' ? ref.height : undefined,
            alt: (ref && typeof ref === 'object' && ref.alt) || altFallback,
        });
    };

    if (event.cover_image_id) {
        push({ id: event.cover_image_id }, event.title);
    }
    if (event.cover_image) {
        push(event.cover_image, event.title);
    }
    if (Array.isArray(event.gallery)) {
        event.gallery.forEach((img, i) => push(img, `${event.title || 'Event'} photo ${i + 1}`));
    }

    return images;
}

/** Best-effort single cover URL, for contexts that only ever show one image (cards, lists). */
export function getCoverImageUrl(event) {
    const images = getEventImages(event);
    return images[0]?.url ?? null;
}
