// src/pages/CategoriesPage.tsx
import React from 'react';
import { Container, Typography, Grid, Card, CardMedia, CardContent, Box } from '@mui/material';

const categories = [
  { id: '1', name: 'Nightlife', imageUrl: 'https://images.unsplash.com/photo-1566737236500-c8ac43014a67?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80', eventCount: 103 },
  { id: '2', name: 'Festival', imageUrl: 'https://images.unsplash.com/photo-1533174072545-7a4b6ad7a6c3?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80', eventCount: 90 },
  { id: '3', name: 'Lifestyle', imageUrl: 'https://images.unsplash.com/photo-1511795409834-ef04bbd61622?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80', eventCount: 71 },
  { id: '4', name: 'Active', imageUrl: 'https://images.unsplash.com/photo-1518611012118-696072aa579a?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80', eventCount: 50 },
  { id: '5', name: 'Music', imageUrl: 'https://images.unsplash.com/photo-1470225620780-dba8ba36b745?ixlib=rb-1.2.1&auto=format&fit=crop&w=1350&q=80', eventCount: 46 },
  { id: '6', name: 'Business', imageUrl: 'https://via.placeholder.com/300x200.png?text=Business', eventCount: 11 },
  { id: '7', name: 'Sports', imageUrl: 'https://via.placeholder.com/300x200.png?text=Sports', eventCount: 11 },
  { id: '8', name: 'School', imageUrl: 'https://via.placeholder.com/300x200.png?text=School', eventCount: 6 },
];

const CategoriesPage: React.FC = () => {
  return (
    <Container maxWidth="xl" sx={{ py: 4 }}>
      <Typography variant="h3" gutterBottom>
        All Categories
      </Typography>
      <Typography variant="subtitle1" gutterBottom>
        {categories.length} Categories
      </Typography>
      <Grid container spacing={3}>
        {categories.map((category) => (
          <Grid item xs={12} sm={6} md={3} key={category.id}>
            <Card sx={{ position: 'relative', borderRadius: '15px' }}>
              <CardMedia
                component="img"
                height="150"
                image={category.imageUrl}
                alt={category.name}
                sx={{ borderRadius: '15px 15px 0 0' }}
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
                  borderRadius: '0 0 15px 15px',
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
  );
};

export default CategoriesPage;
