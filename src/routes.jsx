import React from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';

// Layouts
import BlankLayout from './components/layout/blank-layout';
import MainLayout from './components/layout/main-layout';

// Auth Pages
import SignIn from './pages/auth/signin';
import SignUp from './pages/auth/signup';
import ForgotPassword from './pages/auth/forgot-password';
import UpdatePassword from './pages/auth/update-password';

// Protected Pages


// Components
import ProtectedRoute from './components/auth/protected-route';

import NotFound from './pages/not-found';
import LandingPage from './pages/landing';
import DocsPage from './pages/docs';

const AppRoutes = () => {
  return (
    <Routes>
      {/* Public routes */}
      <Route element={<BlankLayout />}>
        <Route exact path="/" element={<LandingPage />} />
        <Route exact path="/docs" element={<DocsPage />} />

        <Route path="/login" element={<SignIn />} />
        <Route path="/signup" element={<SignUp />} />
        <Route path="/password-reset" element={<ForgotPassword />} />
        <Route path="/update-password" element={<UpdatePassword />} />
        <Route path="*" element={<NotFound />} />
      </Route>

      {/* Protected routes */}
      <Route element={<MainLayout />}>

        <Route path="*" element={<ProtectedRoute><NotFound /></ProtectedRoute>} />
      </Route>
    </Routes>
  );
};

export default AppRoutes;