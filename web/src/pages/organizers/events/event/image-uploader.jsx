import React, { useCallback, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { UploadCloud, X, Star, ChevronUp, ChevronDown, ImageOff, AlertCircle } from 'lucide-react';
import { cn } from '@/lib/utils';
import { images as imagesApi, events as eventsApi } from '@/lib/api';

const MAX_BYTES = 8 * 1024 * 1024; // 8MB — mirrors the server-side cap
const ACCEPTED_TYPES = ['image/png', 'image/jpeg', 'image/webp'];
const ACCEPTED_LABEL = 'PNG, JPEG or WebP, up to 8MB';

function validateFile(file) {
    if (!ACCEPTED_TYPES.includes(file.type)) {
        return `"${file.name}" isn't a supported image type — ${ACCEPTED_LABEL}.`;
    }
    if (file.size > MAX_BYTES) {
        return `"${file.name}" is too large (${(file.size / 1024 / 1024).toFixed(1)}MB) — max is 8MB.`;
    }
    return null;
}

/**
 * Cover + gallery image manager for one event. Wired to the real upload
 * endpoint (`POST /api/events/{id}/images`, multipart) with drag-and-drop,
 * per-file progress, inline validation errors (oversize / wrong type never
 * fail silently), delete, reorder (best-effort — see note on handleReorder),
 * and cover selection. Used by both the create wizard's images step and the
 * event editor's Images page, so upload/delete/reorder behaviour can't
 * drift between the two entry points.
 */
const ImageUploader = ({ eventId, images, coverImageId, onImagesChange, onCoverChange, disabled }) => {
    const [uploads, setUploads] = useState([]); // [{key, name, progress, error}]
    const [dragOver, setDragOver] = useState(false);
    const inputRef = useRef(null);

    const uploadFiles = useCallback(
        async (fileList) => {
            const files = Array.from(fileList || []);
            if (files.length === 0 || !eventId) return;

            for (const file of files) {
                const key = `${file.name}-${file.size}-${Date.now()}-${Math.random()}`;
                const validationError = validateFile(file);
                if (validationError) {
                    setUploads((prev) => [...prev, { key, name: file.name, progress: 0, error: validationError }]);
                    continue;
                }

                setUploads((prev) => [...prev, { key, name: file.name, progress: 0, error: null }]);

                try {
                    const result = await imagesApi.upload(eventId, file, {
                        onProgress: (pct) => setUploads((prev) => prev.map((u) => (u.key === key ? { ...u, progress: pct } : u))),
                    });
                    const image = result?.image ?? result;
                    onImagesChange?.((current) => [...current, image]);
                    setUploads((prev) => prev.filter((u) => u.key !== key));
                } catch (err) {
                    setUploads((prev) => prev.map((u) => (u.key === key ? { ...u, error: err.message || 'Upload failed' } : u)));
                }
            }
        },
        [eventId, onImagesChange],
    );

    const handleDrop = (e) => {
        e.preventDefault();
        setDragOver(false);
        if (disabled) return;
        uploadFiles(e.dataTransfer?.files);
    };

    const dismissUpload = (key) => setUploads((prev) => prev.filter((u) => u.key !== key));

    const handleDelete = async (imageId) => {
        try {
            await imagesApi.remove(imageId);
            onImagesChange?.((current) => current.filter((img) => img.id !== imageId));
            if (imageId === coverImageId) onCoverChange?.(null);
        } catch (err) {
            setUploads((prev) => [...prev, { key: `delete-err-${imageId}-${Date.now()}`, name: 'Delete', progress: 0, error: err.message || 'Could not delete image.' }]);
        }
    };

    // There's no dedicated "reorder gallery" endpoint in the documented API
    // (gallery order isn't part of the contract yet) — reordering is applied
    // optimistically to local state immediately, and persisted best-effort
    // via a PATCH the server can ignore harmlessly if it doesn't recognise
    // the field yet. The visual order is always what this session sees,
    // regardless of whether persistence lands.
    const handleReorder = (index, direction) => {
        onImagesChange?.((current) => {
            const next = [...current];
            const target = index + direction;
            if (target < 0 || target >= next.length) return current;
            [next[index], next[target]] = [next[target], next[index]];
            if (eventId) {
                eventsApi.update(eventId, { gallery_order: next.map((img) => img.id) }).catch(() => {
                    // best-effort persistence only — see comment above
                });
            }
            return next;
        });
    };

    return (
        <div className="space-y-4">
            <div
                onDragOver={(e) => {
                    e.preventDefault();
                    if (!disabled) setDragOver(true);
                }}
                onDragLeave={() => setDragOver(false)}
                onDrop={handleDrop}
                className={cn(
                    'flex flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed px-6 py-10 text-center transition-colors',
                    dragOver ? 'border-primary bg-primary/5' : 'border-border',
                    disabled && 'pointer-events-none opacity-50',
                )}
            >
                <UploadCloud className="h-8 w-8 text-muted-foreground" aria-hidden="true" />
                <p className="text-sm font-medium">Drag and drop images here</p>
                <p className="text-xs text-muted-foreground">{ACCEPTED_LABEL}</p>
                <Button type="button" variant="outline" size="sm" className="mt-2" onClick={() => inputRef.current?.click()} disabled={disabled}>
                    Choose files
                </Button>
                <input
                    ref={inputRef}
                    type="file"
                    accept={ACCEPTED_TYPES.join(',')}
                    multiple
                    className="sr-only"
                    onChange={(e) => {
                        uploadFiles(e.target.files);
                        e.target.value = '';
                    }}
                    aria-label="Upload images"
                />
            </div>

            {uploads.length > 0 && (
                <ul className="space-y-2">
                    {uploads.map((u) => (
                        <li key={u.key} className="rounded-lg border border-border p-3">
                            <div className="flex items-center justify-between gap-2 text-sm">
                                <span className="truncate">{u.name}</span>
                                <button type="button" onClick={() => dismissUpload(u.key)} className="text-muted-foreground hover:text-foreground" aria-label={`Dismiss ${u.name}`}>
                                    <X className="h-4 w-4" />
                                </button>
                            </div>
                            {u.error ? (
                                <p className="mt-1 flex items-center gap-1.5 text-sm text-destructive">
                                    <AlertCircle className="h-3.5 w-3.5 shrink-0" />
                                    {u.error}
                                </p>
                            ) : (
                                <Progress value={u.progress} className="mt-2 h-1.5" />
                            )}
                        </li>
                    ))}
                </ul>
            )}

            {images.length === 0 ? (
                <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-border py-8 text-center text-muted-foreground">
                    <ImageOff className="h-6 w-6" />
                    <p className="text-sm">No images yet. Add a cover image and a few gallery shots to bring the listing to life.</p>
                </div>
            ) : (
                <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
                    {images.map((img, index) => {
                        const isCover = img.id === coverImageId;
                        return (
                            <div key={img.id} className="group relative overflow-hidden rounded-lg border border-border">
                                <div className="aspect-[4/3] w-full bg-muted">
                                    <img src={imagesApi.url(img.id)} alt="" className="h-full w-full object-cover" loading="lazy" />
                                </div>
                                {isCover && (
                                    <span className="absolute left-2 top-2 flex items-center gap-1 rounded-full bg-primary px-2 py-0.5 text-xs font-medium text-primary-foreground">
                                        <Star className="h-3 w-3 fill-current" />
                                        Cover
                                    </span>
                                )}
                                <div className="absolute inset-x-0 bottom-0 flex items-center justify-between gap-1 bg-gradient-to-t from-black/70 to-transparent p-1.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
                                    <div className="flex gap-1">
                                        <Button
                                            type="button"
                                            size="icon"
                                            variant="ghost"
                                            className="h-7 w-7 text-white hover:bg-white/20 hover:text-white"
                                            onClick={() => handleReorder(index, -1)}
                                            disabled={index === 0}
                                            aria-label="Move earlier"
                                        >
                                            <ChevronUp className="h-4 w-4" />
                                        </Button>
                                        <Button
                                            type="button"
                                            size="icon"
                                            variant="ghost"
                                            className="h-7 w-7 text-white hover:bg-white/20 hover:text-white"
                                            onClick={() => handleReorder(index, 1)}
                                            disabled={index === images.length - 1}
                                            aria-label="Move later"
                                        >
                                            <ChevronDown className="h-4 w-4" />
                                        </Button>
                                    </div>
                                    <div className="flex gap-1">
                                        {!isCover && (
                                            <Button
                                                type="button"
                                                size="icon"
                                                variant="ghost"
                                                className="h-7 w-7 text-white hover:bg-white/20 hover:text-white"
                                                onClick={() => onCoverChange?.(img.id)}
                                                title="Set as cover image"
                                                aria-label="Set as cover image"
                                            >
                                                <Star className="h-4 w-4" />
                                            </Button>
                                        )}
                                        <Button
                                            type="button"
                                            size="icon"
                                            variant="ghost"
                                            className="h-7 w-7 text-white hover:bg-white/20 hover:text-white"
                                            onClick={() => handleDelete(img.id)}
                                            title="Delete image"
                                            aria-label="Delete image"
                                        >
                                            <X className="h-4 w-4" />
                                        </Button>
                                    </div>
                                </div>
                            </div>
                        );
                    })}
                </div>
            )}
        </div>
    );
};

export default ImageUploader;
