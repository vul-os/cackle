import React from 'react';
import { AppBar, Toolbar, Typography, Link, Box, Container, Select, MenuItem, IconButton } from '@mui/material';
import TwitterIcon from '@mui/icons-material/Twitter';
import InstagramIcon from '@mui/icons-material/Instagram';
import FacebookIcon from '@mui/icons-material/Facebook';
import TranslateIcon from '@mui/icons-material/Translate';
import ConfirmationNumberIcon from '@mui/icons-material/ConfirmationNumber';

const Footer = () => {
  return (
    <AppBar position="static" color="default" sx={{ 
      top: 'auto', 
      bottom: 0, 
      backgroundColor: '#f5f5f5', 
      boxShadow: 'none',
      py: 4
    }}>
      <Container maxWidth="lg">
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 3 }}>
          <ConfirmationNumberIcon sx={{ fontSize: 40, color: 'text.secondary' }} />
          <Box sx={{ display: 'flex', gap: 2 }}>
            <IconButton color="inherit" aria-label="Twitter" sx={{ color: 'text.secondary' }}>
              <TwitterIcon />
            </IconButton>
            <IconButton color="inherit" aria-label="Instagram" sx={{ color: 'text.secondary' }}>
              <InstagramIcon />
            </IconButton>
            <IconButton color="inherit" aria-label="Facebook" sx={{ color: 'text.secondary' }}>
              <FacebookIcon />
            </IconButton>
          </Box>
        </Box>
        <Toolbar sx={{ 
          flexWrap: 'wrap', 
          justifyContent: 'space-between',
          padding: 0
        }}>
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start' }}>
            <Link href="#" color="inherit" underline="none" sx={{ my: 1, color: 'text.secondary' }}>
              GO CASHLESS / SELL TICKETS
            </Link>
            <Link href="#" color="inherit" underline="none" sx={{ my: 1, color: 'text.secondary' }}>
              HELP
            </Link>
            <Link href="#" color="inherit" underline="none" sx={{ my: 1, color: 'text.secondary' }}>
              CONTACT US
            </Link>
          </Box>
          
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start' }}>
            <Link href="#" color="inherit" underline="none" sx={{ my: 1, color: 'text.secondary' }}>
              TERMS & CONDITIONS
            </Link>
            <Link href="#" color="inherit" underline="none" sx={{ my: 1, color: 'text.secondary' }}>
              PRIVACY POLICY
            </Link>
            <Link href="#" color="inherit" underline="none" sx={{ my: 1, color: 'text.secondary' }}>
              LEGAL
            </Link>
          </Box>
          
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', justifyContent: 'flex-end', height: '100%' }}>
            <Box sx={{ display: 'flex', alignItems: 'center' }}>
              <TranslateIcon sx={{ mr: 1, color: 'text.secondary' }} />
              <Select
                value="English"
                size="small"
                sx={{ 
                  minWidth: 120,
                  color: 'text.secondary',
                  '.MuiOutlinedInput-notchedOutline': { borderColor: 'text.secondary' }
                }}
              >
                <MenuItem value="English">English</MenuItem>
                {/* Add other language options here */}
              </Select>
            </Box>
          </Box>
        </Toolbar>
      </Container>
    </AppBar>
  );
};

export default Footer;