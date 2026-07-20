import { useState } from 'react';

const EMPTY_FORM = {
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
    // the organiser picks explicitly (see schedule-venue.jsx/details.jsx).
    currency: '',
    status: 'draft',
};

export const useEventForm = () => {
    const [hasChanges, setHasChanges] = useState(false);
    const [editForm, setEditForm] = useState(EMPTY_FORM);
    const [dateRange, setDateRange] = useState({ from: null, to: null });

    const handleInputChange = (field, value) => {
        setEditForm((prev) => ({ ...prev, [field]: value }));
        setHasChanges(true);
    };

    const initializeForm = (data) => {
        setEditForm({
            id: data.id,
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
