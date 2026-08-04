import React from 'react';
import { BrowserRouter as Router } from 'react-router-dom';
import { AuthProvider } from './context/use-auth';
import { CartProvider } from './context/use-cart';
import { ThemeProvider } from '@/components/theme-provider';
import { Toaster } from '@/components/ui/toaster';
import AppRoutes from './routes';

const App = () => {
    return (
        <ThemeProvider defaultTheme="system" storageKey="cackle-ui-theme">
            <Router>
                {/* AuthProvider needs router context (it redirects on 401),
                    so it must live inside <Router>, not wrap it. */}
                <AuthProvider>
                    <CartProvider>
                        <AppRoutes />
                        <Toaster />
                    </CartProvider>
                </AuthProvider>
            </Router>
        </ThemeProvider>
    );
};

export default App;
