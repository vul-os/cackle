import React from 'react';
import { Grid, Card, CardMedia, CardContent, Typography, Button, Container, Box, Link as MuiLink, Paper } from '@mui/material';
import { Link } from 'react-router-dom';

// ... (keep your existing Event interface and other imports)

interface Category {
  id: string;
  name: string;
  imageUrl: string;
  eventCount: number;
}

// Sample category data - replace with your actual data
const categories: Category[] = [
  { id: '1', name: 'Nightlife', imageUrl: 'https://example.com/nightlife.jpg', eventCount: 103 },
  { id: '2', name: 'Festival', imageUrl: 'https://example.com/festival.jpg', eventCount: 88 },
  { id: '3', name: 'Lifestyle', imageUrl: 'https://example.com/lifestyle.jpg', eventCount: 70 },
  { id: '4', name: 'Active', imageUrl: 'https://example.com/active.jpg', eventCount: 50 },
  { id: '5', name: 'Music', imageUrl: 'https://example.com/music.jpg', eventCount: 46 },
];

const HomePage: React.FC = () => {
  return (
    <Container maxWidth="xl">
      {/* Organizer Banner */}
      <Paper 
        elevation={0} 
        sx={{ 
          bgcolor: '#8e44ad', 
          color: 'white', 
          p: 2, 
          my: 4, 
          borderRadius: 2,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center'
        }}
      >
        <Typography variant="h6" component="div">
          Organise events? Supercharge your next event with Howler. <MuiLink component={Link} to="/learn-more" color="inherit">Learn more &gt;</MuiLink>
        </Typography>
        <Box sx={{ display: 'flex', alignItems: 'center' }}>
          <img src="/path-to-howler-logo.png" alt="Howler" style={{ height: 30, marginRight: 10 }} />
          <Button variant="contained" color="secondary">
            ORGANISER
          </Button>
        </Box>
      </Paper>

      {/* Categories Section */}
      <Box sx={{ my: 4 }}>
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
      </Box>

      {/* ... (keep your existing Featured Events and Upcoming Events sections) */}
    </Container>
  );
};

export default HomePage;