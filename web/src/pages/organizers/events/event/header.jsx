import React from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { ArrowLeft, Ticket, BarChart3, Globe, Loader2, Users, Image as ImageIcon, Copy, Receipt, Split } from 'lucide-react';

export const EventPageHeader = ({ editForm, handleInputChange, navigate, isSubmitting, onPublish, isPublishing, onDuplicate, isDuplicating }) => {
    return (
        <div className="mb-8">
            <Button variant="ghost" onClick={() => navigate('/admin/events')} className="mb-4 h-11">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Events
            </Button>

            {/* The title gets its OWN row at every width.
                It used to share a `md:flex-row` line with the action group. The
                title column was `flex-1` (flex-basis 0) and the action group was
                a 7-button `flex-wrap` box whose flex base size is its
                single-line max-content width — so the row was over-full, every
                pixel of shrink came out of the zero-basis title, and the input
                rendered 18px wide at both 768 and 1440: an organiser editing an
                event could not see which event they were editing. Stacking is
                the fix that holds for a short title AND a long one, because the
                title's width no longer depends on how many actions exist. */}
            <div className="mb-6 flex flex-col gap-4">
                <div className="flex items-center gap-3">
                    <Input
                        value={editForm.title}
                        onChange={(e) => handleInputChange('title', e.target.value)}
                        className="h-auto min-w-0 flex-1 border-transparent bg-transparent p-2 font-display text-2xl font-bold hover:border-border focus-visible:ring-1 md:text-3xl"
                        placeholder="Event Title"
                        aria-label="Event title"
                        disabled={isSubmitting}
                    />
                    <Badge className="shrink-0" variant={editForm.status === 'published' ? 'default' : 'secondary'}>
                        {editForm.status}
                    </Badge>
                </div>
                <div className="flex flex-wrap gap-2">
                    {editForm.status !== 'published' && editForm.id && (
                        <Button variant="outline" className="h-11" onClick={onPublish} disabled={isPublishing}>
                            {isPublishing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Globe className="mr-2 h-4 w-4" />}
                            Publish
                        </Button>
                    )}
                    <Button variant="outline" className="h-11" onClick={() => navigate(`/admin/events/${editForm.id}/stats`)} disabled={!editForm.id}>
                        <BarChart3 className="mr-2 h-4 w-4" />
                        Stats
                    </Button>
                    <Button variant="outline" className="h-11" onClick={() => navigate(`/admin/events/${editForm.id}/attendees`)} disabled={!editForm.id}>
                        <Users className="mr-2 h-4 w-4" />
                        Attendees
                    </Button>
                    <Button
                        variant="outline"
                        className="h-11"
                        onClick={() => navigate(`/admin/events/${editForm.id}/admissions`)}
                        disabled={!editForm.id}
                        title="Cross-gate double admissions, detected after sync — never prevented"
                    >
                        <Split className="mr-2 h-4 w-4" />
                        Admissions
                    </Button>
                    <Button variant="outline" className="h-11" onClick={() => navigate(`/admin/events/${editForm.id}/orders`)} disabled={!editForm.id}>
                        <Receipt className="mr-2 h-4 w-4" />
                        Orders
                    </Button>
                    <Button variant="outline" className="h-11" onClick={() => navigate(`/admin/events/${editForm.id}/tickets`)} disabled={!editForm.id}>
                        <Ticket className="mr-2 h-4 w-4" />
                        Ticket Types
                    </Button>
                    <Button variant="outline" className="h-11" onClick={() => navigate(`/admin/events/${editForm.id}/images`)} disabled={!editForm.id}>
                        <ImageIcon className="mr-2 h-4 w-4" />
                        Images
                    </Button>
                    {onDuplicate && (
                        <Button variant="outline" className="h-11" onClick={onDuplicate} disabled={!editForm.id || isDuplicating}>
                            {isDuplicating ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Copy className="mr-2 h-4 w-4" />}
                            Duplicate
                        </Button>
                    )}
                </div>
            </div>
        </div>
    );
};

export default EventPageHeader;
