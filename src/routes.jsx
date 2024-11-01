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
import AcceptInvite from './pages/auth/accept-invite';


// Components
import ProtectedRoute from './components/auth/protected-route';

import NotFound from './pages/not-found';
import LandingPage from './pages/visitor/landing';
import DocsPage from './pages/visitor/docs';
import HomePage from './pages/organizers/home';
import EventsPage from './pages/organizers/events';
import SettingsPage from './pages/organizers/settings';
import EventPage from './pages/organizers/events/event';

import VisitorEventPage from './pages/visitor/events/event';
import EventTicketTypes from './pages/organizers/events/event/tickets';


const AppRoutes = () => {
  return (
    <Routes>
      <Route element={<BlankLayout />}>
        {/* Public routes */}
        <Route exact path="/" element={<LandingPage />} />
        <Route exact path="/docs" element={<DocsPage />} />
        <Route exact path="/events/" element={<VisitorEventPage />} />
        <Route path="/login" element={<SignIn />} />
        <Route path="/signup" element={<SignUp />} />
        <Route path="/password-reset" element={<ForgotPassword />} />
        <Route path="/update-password" element={<UpdatePassword />} />
        <Route path="*" element={<NotFound />} />

        {/* Protected routes */}
        <Route path="/accept-invite" element={<ProtectedRoute><AcceptInvite /></ProtectedRoute>} />
      </Route>

      {/* Protected routes */}
      <Route element={<MainLayout />}>
        <Route path="/home" element={<ProtectedRoute><HomePage /></ProtectedRoute>} />
        <Route path="/admin/events" element={<ProtectedRoute><EventsPage /></ProtectedRoute>} />
        <Route path="/admin/events/:id" element={<ProtectedRoute><EventPage /></ProtectedRoute>} />
        <Route path="/admin/events/:id/tickets" element={<ProtectedRoute><EventTicketTypes /></ProtectedRoute>} />

        <Route path="/settings" element={<ProtectedRoute><SettingsPage /></ProtectedRoute>} />
        <Route path="*" element={<ProtectedRoute><NotFound /></ProtectedRoute>} />
      </Route>

    </Routes>
  );
};

export default AppRoutes;