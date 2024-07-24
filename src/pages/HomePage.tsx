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

const featuredEvents: Event[] = [
  {
    id: '1',
    title: 'M FEST',
    description: '26-27 OCTOBER 2024 KYALAMI GRAND PRIX CIRCUIT',
    imageUrl: 'https://images.unsplash.com/photo-1501281668745-f7f57925c3b4?ixlib=rb-1.2.1&auto=format&fit=crop&w=1567&q=80',
  },
  {
    id: '2',
    title: 'Summer Music Festival',
    description: 'A weekend of live performances under the sun',
    imageUrl: 'https://images.unsplash.com/photo-1533174072545-7a4b6ad7a6c3?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80',
  },
  {
    id: '3',
    title: 'Tech Conference 2024',
    description: 'Exploring the future of technology',
    imageUrl: 'https://images.unsplash.com/photo-1540575467063-178a50c2df87?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80',
  },
  {
    id: '4',
    title: 'Food & Wine Expo',
    description: 'Taste the best cuisines from around the world',
    imageUrl: 'https://images.unsplash.com/photo-1414235077428-338989a2e8c0?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80',
  },
];

const upcomingEvents: UpcomingEvent[] = [
  {
    id: '1',
    title: 'elrow Ushuaia 24 July 2024',
    venue: 'Ushuaia Beach Club',
    date: '24 Jul 2024 CEST (+02:00)',
    price: 'From €45.00',
    imageUrl: 'https://images.unsplash.com/photo-1574391884720-bbc3740c59d1?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80',
  },
  {
    id: '2',
    title: 'Comedy Night at Time Out Market Cape Town',
    venue: 'Time Out Studio at Time Out Market Cape Town',
    date: '25 Jul 2024 SAST (+02:00)',
    price: 'Tickets R150.00',
    imageUrl: 'https://images.unsplash.com/photo-1585699324551-f6c309eedeca?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80',
  },
  {
    id: '3',
    title: 'The Influencer Party',
    venue: 'Katzys Live Rosebank',
    date: '26 Jul - 27 Jul SAST (+02:00)',
    price: 'From R100.00',
    imageUrl: 'https://images.unsplash.com/photo-1496024840928-4c417adf211d?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80',
  },
];

const categories: Category[] = [
  { id: '1', name: 'Nightlife', imageUrl: 'https://images.unsplash.com/photo-1566737236500-c8ac43014a67?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80', eventCount: 103 },
  { id: '2', name: 'Festival', imageUrl: 'https://images.unsplash.com/photo-1533174072545-7a4b6ad7a6c3?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80', eventCount: 88 },
  { id: '3', name: 'Lifestyle', imageUrl: 'https://images.unsplash.com/photo-1511795409834-ef04bbd61622?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80', eventCount: 70 },
  { id: '4', name: 'Active', imageUrl: 'https://images.unsplash.com/photo-1518611012118-696072aa579a?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80', eventCount: 50 },
  { id: '5', name: 'Music', imageUrl: 'https://images.unsplash.com/photo-1470225620780-dba8ba36b745?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80', eventCount: 46 },
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
                <img src="https://images.unsplash.com/photo-1540039155733-5bb30b53aa14?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80" alt="Event 1" style={{ width: '30%', height: '30%', objectFit: 'cover', borderRadius: '8px' }} />
                <img src="https://images.unsplash.com/photo-1501281668745-f7f57925c3b4?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80" alt="Event 2" style={{ width: '30%', height: '30%', objectFit: 'cover', borderRadius: '8px' }} />
                {/* Add more images as needed */}
              </Box>
            </Grid>
          </Grid>
        </Container>
      </Box>

      {/* Navigation */}
      <Box sx={{ borderTop: '1px solid rgba(255,255,255,0.1)', borderBottom: '1px solid rgba(255,255,255,0.1)', py: 2 }}>
        <Container maxWidth="xl">
          <Grid container spacing={4} justifyContent="center">
            <Grid item>
              <MuiLink component={Link} to="/" color="inherit" sx={{ textDecoration: 'none', fontWeight: 'bold' }}>FEATURED</MuiLink>
            </Grid>
            <Grid item>
              <MuiLink component={Link} to="/categories" color="inherit" sx={{ textDecoration: 'none', fontWeight: 'bold' }}>CATEGORIES</MuiLink>
            </Grid>
            <Grid item>
              <MuiLink component={Link} to="/artists" color="inherit" sx={{ textDecoration: 'none', fontWeight: 'bold' }}>ARTISTS</MuiLink>
            </Grid>
            <Grid item>
              <MuiLink component={Link} to="/organisers" color="inherit" sx={{ textDecoration: 'none', fontWeight: 'bold' }}>ORGANISERS</MuiLink>
            </Grid>
            <Grid item>
              <MuiLink component={Link} to="/venues" color="inherit" sx={{ textDecoration: 'none', fontWeight: 'bold' }}>VENUES</MuiLink>
            </Grid>
          </Grid>
        </Container>
      </Box>

      {/* Featured Events */}
      <Box sx={{ py: 8 }}>
        <Container maxWidth="xl">
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Typography variant="h4" gutterBottom>
              Featured Events
            </Typography>
            <MuiLink component={Link} to="/all-events" color="inherit" sx={{ textDecoration: 'none', fontWeight: 'bold' }}>
              See All
            </MuiLink>
          </Box>
          <Grid container spacing={4}>
            {featuredEvents.map(event => (
              <Grid item key={event.id} xs={12} md={3}>
                <Card>
                  <CardMedia
                    component="img"
                    height="140"
                    image={event.imageUrl}
                    alt={event.title}
                  />
                  <CardContent>
                    <Typography variant="h6">{event.title}</Typography>
                    <Typography variant="body2" color="textSecondary">
                      {event.description}
                    </Typography>
                  </CardContent>
                </Card>
              </Grid>
            ))}
          </Grid>
        </Container>
      </Box>

      {/* Upcoming Events */}
      <Box sx={{ py: 8, bgcolor: '#f5f5f5', color: 'black' }}>
        <Container maxWidth="xl">
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Typography variant="h4" gutterBottom>
              Upcoming Events
            </Typography>
            <MuiLink component={Link} to="/all-events" color="inherit" sx={{ textDecoration: 'none', fontWeight: 'bold' }}>
              See All
            </MuiLink>
          </Box>
          <Grid container spacing={4}>
            {upcomingEvents.map(event => (
              <Grid item key={event.id} xs={12} md={4}>
                <Card>
                  <CardMedia
                    component="img"
                    height="140"
                    image={event.imageUrl}
                    alt={event.title}
                  />
                  <CardContent>
                    <Typography variant="h6">{event.title}</Typography>
                    <Typography variant="body2" color="textSecondary">
                      {event.venue}<br />
                      {event.date}<br />
                      {event.price}
                    </Typography>
                  </CardContent>
                </Card>
              </Grid>
            ))}
          </Grid>
        </Container>
      </Box>

      {/* Event Categories */}
      <Box sx={{ py: 8 }}>
        <Container maxWidth="xl">
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Typography variant="h4" gutterBottom>
              Event Categories
            </Typography>
            <MuiLink component={Link} to="/all-events" color="inherit" sx={{ textDecoration: 'none', fontWeight: 'bold' }}>
              See All
            </MuiLink>
          </Box>
          <Grid container spacing={4}>
            {categories.map(category => (
              <Grid item key={category.id} xs={12} md={3}>
                <Card>
                  <CardMedia
                    component="img"
                    height="140"
                    image={category.imageUrl}
                    alt={category.name}
                  />
                  <CardContent>
                    <Typography variant="h6">{category.name}</Typography>
                    <Typography variant="body2" color="textSecondary">
                      {category.eventCount} events
                    </Typography>
                  </CardContent>
                </Card>
              </Grid>
            ))}
          </Grid>
        </Container>
      </Box>

      {/* Footer */}
      <Box sx={{ py: 4, textAlign: 'center', bgcolor: '#121212', color: 'white' }}>
        <Typography variant="body2">&copy; 2024 Howler. All rights reserved.</Typography>
      </Box>
    </Box>
  );
}

export default HomePage;
