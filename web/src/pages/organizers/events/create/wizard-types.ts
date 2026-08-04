/**
 * The create-event wizard's own event shape, shared by index.tsx and every
 * step under ./steps/. It is a superset of CreateEventInput/UpdateEventInput
 * (lib/api.ts) held as editable strings — same reasoning as
 * pages/organizers/events/event/event-form-hook.ts's EventFormState, and
 * deliberately not the same type: the wizard never carries an `id`,
 * `cover_image`/`cover_image_id` or `status` field of its own (cover image
 * and id are tracked as separate wizard state; status is implicit —
 * everything the wizard holds is a draft until Publish).
 */
export interface WizardEvent {
    title: string;
    category: string;
    summary: string;
    description: string;
    venue_name: string;
    address: string;
    lat: string | number;
    lng: string | number;
    currency: string;
    starts_at: string;
    ends_at: string;
}
