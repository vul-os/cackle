import React from 'react';
import { AppBar, Toolbar, Typography, Button, Box, Container, TextField, InputAdornment, Grid, Card, CardMedia, CardContent, Link as MuiLink } from '@mui/material';
import SearchIcon from '@mui/icons-material/Search';
import TranslateIcon from '@mui/icons-material/Translate';
import { Link } from 'react-router-dom';

interface Event {
  id: string;
  title: string;
  description: string;
  imageUrl: string;
}

interface UpcomingEvent {
  id: string;
  title: string;
  venue: string;
  date: string;
  price: string;
  imageUrl: string;
}

interface Category {
  id: string;
  name: string;
  imageUrl: string;
  eventCount: number;
}

// Sample data - replace with your actual data
const featuredEvents: Event[] = [
  {
    id: '1',
    title: 'M FEST',
    description: '26-27 OCTOBER 2024 KYALAMI GRAND PRIX CIRCUIT',
    imageUrl: 'https://example.com/mfest.jpg',
  },
  // Add more featured events...
];

const upcomingEvents: UpcomingEvent[] = [
  {
    id: '1',
    title: 'elrow Ushuaia 24 July 2024',
    venue: 'Ushuaia Beach Club',
    date: '24 Jul 2024 CEST (+02:00)',
    price: 'From €45.00',
    imageUrl: 'https://example.com/elrow-ushuaia.jpg',
  },
  {
    id: '2',
    title: 'Comedy Night at Time Out Market Cape Town',
    venue: 'Time Out Studio at Time Out Market Cape Town',
    date: '25 Jul 2024 SAST (+02:00)',
    price: 'Tickets R150.00',
    imageUrl: 'https://example.com/comedy-night.jpg',
  },
  {
    id: '3',
    title: 'The Influencer Party',
    venue: 'Katzys Live Rosebank',
    date: '26 Jul - 27 Jul SAST (+02:00)',
    price: 'From R100.00',
    imageUrl: 'https://example.com/influencer-party.jpg',
  },
];

const categories: Category[] = [
  { id: '1', name: 'Nightlife', imageUrl: 'https://example.com/nightlife.jpg', eventCount: 103 },
  { id: '2', name: 'Festival', imageUrl: 'https://example.com/festival.jpg', eventCount: 88 },
  { id: '3', name: 'Lifestyle', imageUrl: 'https://example.com/lifestyle.jpg', eventCount: 70 },
  { id: '4', name: 'Active', imageUrl: 'https://example.com/active.jpg', eventCount: 50 },
  { id: '5', name: 'Music', imageUrl: 'https://example.com/music.jpg', eventCount: 46 },
];

const HomePage: React.FC = () => {
  return (
    <Box sx={{ minHeight: '100vh', bgcolor: '#121212', color: 'white' }}>
      {/* Header */}
      <AppBar position="static" color="transparent" elevation={0}>
        <Toolbar>
          <Typography variant="h6" component="div" sx={{ flexGrow: 1 }}>
            <img src="/path-to-howler-logo.png" alt="Howler" style={{ height: 40 }} />
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <TranslateIcon sx={{ mr: 1, color: 'white' }} />
            <Button color="inherit">Log In</Button>
            <Button variant="contained" sx={{ ml: 2, bgcolor: '#E91E63', color: 'white' }}>Sign Up</Button>
          </Box>
        </Toolbar>
      </AppBar>

      {/* Purple Banner */}
      <Box sx={{ bgcolor: '#8e44ad', color: 'white', py: 1, textAlign: 'center' }}>
        <Typography variant="body1">
          Hosting an event? Sell more (than) tickets with Howler
          <Button variant="contained" size="small" sx={{ ml: 2, bgcolor: 'white', color: '#8e44ad' }}>
            GET STARTED
          </Button>
        </Typography>
      </Box>

      {/* Hero Section */}
      <Box sx={{ py: 8 }}>
        <Container maxWidth="xl">
          <Grid container spacing={4}>
            <Grid item xs={12} md={6}>
              <Typography variant="h2" component="h1" gutterBottom sx={{ fontWeight: 'bold' }}>
                At the heart of the best events
              </Typography>
              <Typography variant="body1" paragraph>
                Less work, more play. Whether you're into online streams, weekend festivals or daytime get-togethers; we have something for you. Find what you're looking for and join the movement.
              </Typography>
              <TextField
                fullWidth
                variant="outlined"
                placeholder="Search events, organisers, venues or artists"
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <SearchIcon />
                    </InputAdornment>
                  ),
                  sx: { bgcolor: 'white', borderRadius: 2 }
                }}
              />
            </Grid>
            <Grid item xs={12} md={6}>
              {/* Image Collage */}
              <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 2, justifyContent: 'flex-end' }}>
                <img src="https://example.com/event1.jpg" alt="Event 1" style={{ width: '30%', height: '30%', objectFit: 'cover', borderRadius: '8px' }} />
                <img src="https://example.com/event2.jpg" alt="Event 2" style={{ width: '30%', height: '30%', objectFit: 'cover', borderRadius: '8px' }} />
                {/* Add more images as needed */}
              </Box>
            </Grid>
          </Grid>
        </Container>
      </Box>

      {/* Navigation */}
      <Box sx={{ borderTop: '1px solid rgba(255,255,255,0.1)', borderBottom: '1px solid rgba(255,255,255,0.1)', py: 2 }}>
        <Container maxWidth="xl">
          <Grid container spacing={2}>
            {['Featured Events', 'Upcoming', 'Categories', 'Artists'].map((item) => (
              <Grid item key={item}>
                <Button sx={{ color: 'white' }}>{item}</Button>
              </Grid>
            ))}
          </Grid>
        </Container>
      </Box>

      {/* Featured Events Section */}
      <Box sx={{ py: 4 }}>
        <Container maxWidth="xl">
          <Typography variant="h4" gutterBottom>
            Featured Events
          </Typography>
          <Typography variant="subtitle1" gutterBottom>
            Our Favourite Picks
          </Typography>
          <Grid container spacing={2}>
            <Grid item xs={12} md={8}>
              <Card sx={{ height: '100%', bgcolor: '#1E1E1E' }}>
                <CardMedia
                  component="img"
                  height="400"
                  image={featuredEvents[0].imageUrl}
                  alt={featuredEvents[0].title}
                />
                <CardContent>
                  <Typography gutterBottom variant="h5" component="div" color="white">
                    {featuredEvents[0].title}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    {featuredEvents[0].description}
                  </Typography>
                </CardContent>
              </Card>
            </Grid>
            <Grid item xs={12} md={4}>
              <Grid container spacing={2} direction="column">
                {featuredEvents.slice(1, 4).map((event) => (
                  <Grid item key={event.id}>
                    <Card sx={{ display: 'flex', height: 120, bgcolor: '#1E1E1E' }}>
                      <CardMedia
                        component="img"
                        sx={{ width: 151 }}
                        image={event.imageUrl}
                        alt={event.title}
                      />
                      <Box sx={{ display: 'flex', flexDirection: 'column', width: '100%' }}>
                        <CardContent sx={{ flex: '1 0 auto' }}>
                          <Typography component="div" variant="h6" color="white">
                            {event.title}
                          </Typography>
                          <Typography variant="subtitle1" color="text.secondary" component="div">
                            {event.description}
                          </Typography>
                        </CardContent>
                      </Box>
                    </Card>
                  </Grid>
                ))}
              </Grid>
            </Grid>
          </Grid>
        </Container>
      </Box>

      {/* Upcoming Events Section */}
      <Box sx={{ py: 4, bgcolor: '#1E1E1E' }}>
        <Container maxWidth="xl">
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
            <div>
              <Typography variant="h4" gutterBottom>
                Upcoming Events
              </Typography>
              <Typography variant="subtitle1">
                Events happening soon
              </Typography>
            </div>
            <MuiLink component={Link} to="/events" color="primary">
              See All &gt;
            </MuiLink>
          </Box>
          <Grid container spacing={3}>
            {upcomingEvents.map((event) => (
              <Grid item xs={12} md={4} key={event.id}>
                <Card sx={{ bgcolor: '#2A2A2A' }}>
                  <CardMedia
                    component="img"
                    height="200"
                    image={event.imageUrl}
                    alt={event.title}
                  />
                  <CardContent>
                    <Typography gutterBottom variant="h6" component="div" color="white">
                      {event.title}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                      {event.venue}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                      {event.date}
                    </Typography>
                    <Typography variant="body2" color="primary" sx={{ fontWeight: 'bold', mt: 1 }}>
                      {event.price}
                    </Typography>
                  </CardContent>
                </Card>
              </Grid>
            ))}
          </Grid>
        </Container>
      </Box>

      {/* Categories Section */}
      <Box sx={{ py: 4 }}>
        <Container maxWidth="xl">
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
            <div>
              <Typography variant="h4" gutterBottom>
                Categories
              </Typography>
              <Typography variant="subtitle1">
                Explore events that match your tastes
              </Typography>
            </div>
            <MuiLink component={Link} to="/categories" color="primary">
              See All &gt;
            </MuiLink>
          </Box>
          <Grid container spacing={2}>
            {categories.map((category) => (
              <Grid item xs={12} sm={6} md={2.4} key={category.id}>
                <Card sx={{ position: 'relative' }}>
                  <CardMedia
                    component="img"
                    height="200"
                    image={category.imageUrl}
                    alt={category.name}
                  />
                  <Box
                    sx={{
                      position: 'absolute',
                      bottom: 0,
                      left: 0,
                      width: '100%',
                      bgcolor: 'rgba(0, 0, 0, 0.54)',
                      color: 'white',
                      padding: '10px',
                    }}
                  >
                    <Typography variant="h6">{category.name}</Typography>
                    <Typography variant="body2">{category.eventCount} Events</Typography>
                  </Box>
                </Card>
              </Grid>
            ))}
          </Grid>
        </Container>
      </Box>
    </Box>
  );
};

export default HomePage;