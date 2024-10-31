import React, { useEffect } from 'react';
import { BrowserRouter as Router } from 'react-router-dom';
import { AuthProvider } from './context/auth-context';
import AppRoutes from './routes';
import { Toaster } from "@/components/ui/toaster"
import { ThemeProvider } from '@/components/theme-provider';

const App = () => {

  return (
      <AuthProvider>
          <ThemeProvider defaultTheme="light" storageKey="vite-ui-theme">
          <Router>
            <AppRoutes />
          </Router>
          <Toaster />
        </ThemeProvider>
      </AuthProvider>
  );
};

export default App;