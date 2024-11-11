import { format } from 'date-fns';

export function formatDate(dateString) {
  if (!dateString) return 'N/A';
  try {
    return format(new Date(dateString), 'EEEE, MMMM d, yyyy');
  } catch (error) {
    return 'N/A';
  }
}

export function formatTime(dateString) {
  if (!dateString) return 'N/A';
  try {
    return format(new Date(dateString), 'h:mm a');
  } catch (error) {
    return 'N/A';
  }
}
