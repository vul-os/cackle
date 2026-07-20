import React from 'react';
import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Loader2 } from 'lucide-react';

/**
 * Confirmation dialog for deleting an event. Shared between the event
 * editor (single-event delete) and the events list (quick delete from a
 * card), so the copy and the "type to confirm" guard never drift between
 * the two entry points.
 */
const DeleteEventDialog = ({ open, onOpenChange, eventTitle, onConfirm, isDeleting }) => {
    return (
        <AlertDialog open={open} onOpenChange={(next) => !isDeleting && onOpenChange(next)}>
            <AlertDialogContent>
                <AlertDialogHeader>
                    <AlertDialogTitle>Delete “{eventTitle || 'this event'}”?</AlertDialogTitle>
                    <AlertDialogDescription>
                        This permanently removes the event. Ticket types are removed with it. If tickets have already been sold,
                        cancel the event instead of deleting it so buyers keep their order history.
                    </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                    <AlertDialogCancel disabled={isDeleting}>Cancel</AlertDialogCancel>
                    <AlertDialogAction
                        onClick={(e) => {
                            e.preventDefault();
                            onConfirm();
                        }}
                        disabled={isDeleting}
                        className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                    >
                        {isDeleting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
                        Delete event
                    </AlertDialogAction>
                </AlertDialogFooter>
            </AlertDialogContent>
        </AlertDialog>
    );
};

export default DeleteEventDialog;
