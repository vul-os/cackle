import React from 'react';
import { useParams } from 'react-router-dom';
import { Typography, Card, CardMedia, CardContent, Button } from '@mui/material';

interface Event {
  id: string;
  title: string;
  description: string;
  imageUrl: string;
  date: string;
  location: string;
}

const events: Event[] = [
  {
    id: '1',
    title: 'Summer Music Festival',
    description: 'A weekend of live music performances featuring top artists from around the world. Join us for an unforgettable experience!',
    imageUrl: 'https://example.com/summer-festival.jpg',
    date: 'August 15-17, 2023',
    location: 'Central Park, New York City',
  },
  // Add more events as needed
];

const EventPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const event = events.find(e => e.id === id);

  if (!event) {
    return <Typography variant="h4">Event not found</Typography>;
  }

  return (
    <Card sx={{ maxWidth: 600, mx: 'auto', mt: 4 }}>
      <CardMedia
        component="img"
        height="300"
        image={event.imageUrl}
        alt={event.title}
      />
      <CardContent>
        <Typography gutterBottom variant="h4" component="div">
          {event.title}
        </Typography>
        <Typography variant="body1" color="text.secondary" paragraph>
          {event.description}
        </Typography>
        <Typography variant="body1" paragraph>
          <strong>Date:</strong> {event.date}
        </Typography>
        <Typography variant="body1" paragraph>
          <strong>Location:</strong> {event.location}
        </Typography>
        <Button variant="contained" color="primary">
          Buy Tickets
        </Button>
      </CardContent>
    </Card>
  );
};

export default EventPage;