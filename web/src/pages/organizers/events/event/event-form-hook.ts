import { useState } from 'react';

/**
 * The event editor's own form shape — a superset of CreateEventInput/
 * UpdateEventInput (lib/api.ts) held as editable strings rather than the
 * API's stricter types, since an <Input> deals in strings and a half-typed
 * coordinate or an empty currency picker has to be representable while the
 * organiser is still filling the form in. `lat`/`lng` hold whichever of the
 * two shapes is currently true: '' before anything is entered, a string
 * while the organiser types, or the number initializeForm loaded off the
 * server — never coerced eagerly, because the coercion (or lack of it) is a
 * decision for whichever step actually submits the form.
 */
export interface EventFormState {
    id: string | null;
    title: string;
    summary: string;
    description: string;
    venue_name: string;
    address: string;
    lat: number | string;
    lng: number | string;
    starts_at: string;
    ends_at: string;
    timezone: string;
    cover_image: string;
    cover_image_id: string | null;
    category: string;
    currency: string;
    status: string;
}

const EMPTY_FORM: EventFormState = {
    id: null,
    title: '',
    summary: '',
    description: '',
    venue_name: '',
    address: '',
    lat: '',
    lng: '',
    starts_at: '',
    ends_at: '',
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    cover_image: '',
    cover_image_id: null,
    category: '',
    // No hardcoded default currency — Cackle has no privileged currency;
    // the organiser picks explicitly (see schedule-venue.tsx/details.tsx).
    currency: '',
    status: 'draft',
};

/** The data this hook can seed a form from — whatever an event GET/create response hands back. */
export interface EventFormSeed {
    id?: string | null;
    title?: string | null;
    summary?: string | null;
    description?: string | null;
    venue_name?: string | null;
    address?: string | null;
    lat?: number | string | null;
    lng?: number | string | null;
    starts_at?: string | null;
    ends_at?: string | null;
    timezone?: string | null;
    cover_image?: string | null;
    cover_image_id?: string | null;
    category?: string | null;
    currency?: string | null;
    status?: string | null;
}

export interface DateRange {
    from: Date | null;
    to: Date | null;
}

export const useEventForm = () => {
    const [hasChanges, setHasChanges] = useState(false);
    const [editForm, setEditForm] = useState<EventFormState>(EMPTY_FORM);
    const [dateRange, setDateRange] = useState<DateRange>({ from: null, to: null });

    const handleInputChange = <K extends keyof EventFormState>(field: K, value: EventFormState[K]) => {
        setEditForm((prev) => ({ ...prev, [field]: value }));
        setHasChanges(true);
    };

    const initializeForm = (data: EventFormSeed) => {
        setEditForm({
            id: data.id ?? null,
            title: data.title ?? '',
            summary: data.summary ?? '',
            description: data.description ?? '',
            venue_name: data.venue_name ?? '',
            address: data.address ?? '',
            lat: data.lat ?? '',
            lng: data.lng ?? '',
            starts_at: data.starts_at ?? '',
            ends_at: data.ends_at ?? '',
            timezone: data.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone,
            cover_image: data.cover_image ?? '',
            cover_image_id: data.cover_image_id ?? null,
            category: data.category ?? '',
            currency: data.currency ?? '',
            status: data.status || 'draft',
        });
        if (data.starts_at && data.ends_at) {
            setDateRange({ from: new Date(data.starts_at), to: new Date(data.ends_at) });
        }
        setHasChanges(false);
    };

    return {
        editForm,
        dateRange,
        setDateRange,
        hasChanges,
        setHasChanges,
        handleInputChange,
        initializeForm,
    };
};
