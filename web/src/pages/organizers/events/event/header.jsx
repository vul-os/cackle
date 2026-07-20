import React from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { ArrowLeft, Ticket, BarChart3, Globe, Loader2, Users, Image as ImageIcon, Copy, Receipt } from 'lucide-react';

export const EventPageHeader = ({ editForm, handleInputChange, navigate, isSubmitting, onPublish, isPublishing, onDuplicate, isDuplicating }) => {
    return (
        <div className="mb-8">
            <Button variant="ghost" onClick={() => navigate('/admin/events')} className="mb-4">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Events
            </Button>

            <div className="mb-6 flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
                <div className="flex flex-1 items-center gap-3">
                    <Input
                        value={editForm.title}
                        onChange={(e) => handleInputChange('title', e.target.value)}
                        className="h-auto border-transparent bg-transparent p-2 font-display text-2xl font-bold hover:border-border focus-visible:ring-1 md:text-3xl"
                        placeholder="Event Title"
                        disabled={isSubmitting}
                    />
                    <Badge variant={editForm.status === 'published' ? 'default' : 'secondary'}>{editForm.status}</Badge>
                </div>
                <div className="flex flex-wrap gap-2">
                    {editForm.status !== 'published' && editForm.id && (
                        <Button variant="outline" onClick={onPublish} disabled={isPublishing}>
                            {isPublishing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Globe className="mr-2 h-4 w-4" />}
                            Publish
                        </Button>
                    )}
                    <Button variant="outline" onClick={() => navigate(`/admin/events/${editForm.id}/stats`)} disabled={!editForm.id}>
                        <BarChart3 className="mr-2 h-4 w-4" />
                        Stats
                    </Button>
                    <Button variant="outline" onClick={() => navigate(`/admin/events/${editForm.id}/attendees`)} disabled={!editForm.id}>
                        <Users className="mr-2 h-4 w-4" />
                        Attendees
                    </Button>
                    <Button variant="outline" onClick={() => navigate(`/admin/events/${editForm.id}/orders`)} disabled={!editForm.id}>
                        <Receipt className="mr-2 h-4 w-4" />
                        Orders
                    </Button>
                    <Button variant="outline" onClick={() => navigate(`/admin/events/${editForm.id}/tickets`)} disabled={!editForm.id}>
                        <Ticket className="mr-2 h-4 w-4" />
                        Ticket Types
                    </Button>
                    <Button variant="outline" onClick={() => navigate(`/admin/events/${editForm.id}/images`)} disabled={!editForm.id}>
                        <ImageIcon className="mr-2 h-4 w-4" />
                        Images
                    </Button>
                    {onDuplicate && (
                        <Button variant="outline" onClick={onDuplicate} disabled={!editForm.id || isDuplicating}>
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
