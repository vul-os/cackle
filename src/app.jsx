import React, { useEffect } from 'react';
import { BrowserRouter as Router } from 'react-router-dom';
import { AuthProvider } from './context/auth-context';
import AppRoutes from './routes';
import { Toaster } from "@/components/ui/toaster"
import { ThemeProvider } from '@/components/theme-provider';
import { CartProvider } from './context/use-cart';
import { OrderProvider } from './context/use-order';

const App = () => {

  return (
      <AuthProvider>
        <CartProvider>
          <OrderProvider>
            <ThemeProvider defaultTheme="light" storageKey="vite-ui-theme">
              <Router>
                <AppRoutes />
              </Router>
              <Toaster />
            </ThemeProvider>
          </OrderProvider>
        </CartProvider>
      </AuthProvider>
  );
};
//

export default App;