import React, { useCallback, useEffect, useRef, useState } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import { Calendar, ChevronLeft, ChevronRight } from 'lucide-react';
import { cn } from '@/lib/utils';

const SWIPE_THRESHOLD_PX = 40;

/**
 * Event cover image + gallery. Degrades gracefully through three states:
 *  - no images: a branded placeholder (no broken-image icon, no network call)
 *  - one image: a plain, static hero image
 *  - 2+ images: a keyboard-navigable, swipeable slider with dot indicators
 *
 * `images` is [{ id, url, alt, width, height }]. `title` is used as the alt
 * fallback and in the placeholder state.
 */
const EventGallery = ({ images = [], title = 'Event', className }) => {
    const [index, setIndex] = useState(0);
    const [direction, setDirection] = useState(0);
    const touchStartX = useRef(null);
    const containerRef = useRef(null);

    const count = images.length;

    const goTo = useCallback(
        (next) => {
            if (count === 0) return;
            const wrapped = ((next % count) + count) % count;
            setDirection(wrapped > index || (index === count - 1 && wrapped === 0) ? 1 : -1);
            setIndex(wrapped);
        },
        [count, index],
    );

    const goNext = useCallback(() => goTo(index + 1), [goTo, index]);
    const goPrev = useCallback(() => goTo(index - 1), [goTo, index]);

    // Reset to the first slide if the image list itself changes identity
    // (e.g. navigating from one event page straight to another).
    useEffect(() => {
        setIndex(0);
        setDirection(0);
    }, [images]);

    const handleKeyDown = useCallback(
        (e) => {
            if (count < 2) return;
            if (e.key === 'ArrowRight') {
                e.preventDefault();
                goNext();
            } else if (e.key === 'ArrowLeft') {
                e.preventDefault();
                goPrev();
            }
        },
        [count, goNext, goPrev],
    );

    const handleTouchStart = (e) => {
        touchStartX.current = e.touches[0]?.clientX ?? null;
    };

    const handleTouchEnd = (e) => {
        if (touchStartX.current === null) return;
        const endX = e.changedTouches[0]?.clientX ?? touchStartX.current;
        const delta = endX - touchStartX.current;
        touchStartX.current = null;
        if (Math.abs(delta) < SWIPE_THRESHOLD_PX) return;
        if (delta < 0) goNext();
        else goPrev();
    };

    // No-image state: a calm branded placeholder, never a broken <img>.
    if (count === 0) {
        return (
            <div
                className={cn(
                    'flex h-full w-full items-center justify-center bg-gradient-to-br from-primary/30 to-primary/5',
                    className,
                )}
                role="img"
                aria-label={`${title} — no image available`}
            >
                <Calendar className="h-20 w-20 text-primary/40" aria-hidden="true" />
            </div>
        );
    }

    // Single-image state: no controls, no slider chrome, just the image.
    if (count === 1) {
        return (
            <div className={cn('h-full w-full', className)}>
                <img
                    src={images[0].url}
                    alt={images[0].alt || title}
                    className="h-full w-full object-cover"
                    loading="eager"
                />
            </div>
        );
    }

    const current = images[index];

    return (
        <div
            ref={containerRef}
            className={cn('group relative h-full w-full overflow-hidden bg-muted outline-none', className)}
            role="region"
            aria-roledescription="carousel"
            aria-label={`${title} — photo gallery`}
            tabIndex={0}
            onKeyDown={handleKeyDown}
            onTouchStart={handleTouchStart}
            onTouchEnd={handleTouchEnd}
        >
            <AnimatePresence initial={false} custom={direction} mode="popLayout">
                <motion.img
                    key={current.id}
                    src={current.url}
                    alt={current.alt || `${title} — photo ${index + 1} of ${count}`}
                    loading={index === 0 ? 'eager' : 'lazy'}
                    custom={direction}
                    initial={{ opacity: 0, x: direction >= 0 ? 48 : -48 }}
                    animate={{ opacity: 1, x: 0 }}
                    exit={{ opacity: 0, x: direction >= 0 ? -48 : 48 }}
                    transition={{ duration: 0.28, ease: 'easeOut' }}
                    className="absolute inset-0 h-full w-full object-cover"
                />
            </AnimatePresence>

            <button
                type="button"
                onClick={goPrev}
                aria-label="Previous photo"
                className="absolute left-3 top-1/2 z-10 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-black/40 text-white opacity-0 backdrop-blur-sm transition-opacity duration-200 hover:bg-black/60 focus-visible:opacity-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-white group-hover:opacity-100 group-focus-within:opacity-100"
            >
                <ChevronLeft className="h-5 w-5" />
            </button>
            <button
                type="button"
                onClick={goNext}
                aria-label="Next photo"
                className="absolute right-3 top-1/2 z-10 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-black/40 text-white opacity-0 backdrop-blur-sm transition-opacity duration-200 hover:bg-black/60 focus-visible:opacity-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-white group-hover:opacity-100 group-focus-within:opacity-100"
            >
                <ChevronRight className="h-5 w-5" />
            </button>

            <div className="absolute bottom-4 left-1/2 z-10 flex -translate-x-1/2 items-center gap-3">
                <div className="flex items-center gap-1.5 rounded-full bg-black/40 px-3 py-1.5 backdrop-blur-sm">
                    {images.map((img, i) => (
                        <button
                            key={img.id}
                            type="button"
                            onClick={() => goTo(i)}
                            aria-label={`Go to photo ${i + 1} of ${count}`}
                            aria-current={i === index}
                            className={cn(
                                'h-1.5 rounded-full transition-all duration-200 focus-visible:outline focus-visible:outline-2 focus-visible:outline-white',
                                i === index ? 'w-5 bg-white' : 'w-1.5 bg-white/50 hover:bg-white/75',
                            )}
                        />
                    ))}
                </div>
                <span className="rounded-full bg-black/40 px-2 py-1 text-xs font-medium text-white backdrop-blur-sm" aria-hidden="true">
                    {index + 1} / {count}
                </span>
            </div>
        </div>
    );
};

export default EventGallery;
