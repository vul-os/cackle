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

// Pages
import NotFound from './pages/not-found';
import LandingPage from './pages/visitor/landing';
import DocsPage from './pages/visitor/docs';
import HomePage from './pages/organizers/home';
import EventsPage from './pages/organizers/events';
import SettingsPage from './pages/organizers/settings';
import EventPage from './pages/organizers/events/event';
import ContactPage from'./pages/visitor/contact';
import PricingPage from './pages/organizers/pricing';
import VisitorEventPage from './pages/visitor/events/event';
import EventTicketsLayout from './pages/organizers/events/event/tickets';
import TicketsView from './pages/organizers/events/event/tickets/tickets-view';
import TicketTypesView from './pages/organizers/events/event/tickets/ticket-types-view';
import ScannerPage from './pages/organizers/scanner';
import CartPage from './pages/visitor/cart';
import CheckoutPage from './pages/visitor/checkout';
import OrderPage from './pages/visitor/orders/order';
import OrdersPage from './pages/visitor/orders';
import PaymentConfirmationPage from './pages/visitor/payment/confirmation';
import AttendeesPage from './pages/organizers/events/event/attendees';



// Ticket Pages
import TicketPage from './pages/visitor/ticket';
import TicketsListPage from './pages/visitor/tickets';
import BankListPage from './pages/organizers/bank-list';
import PayoutsPage from './pages/organizers/events/event/payouts';

const AppRoutes = () => {
  return (
    <Routes>
      <Route element={<BlankLayout />}>
        {/* Public routes */}
        <Route exact path="/" element={<LandingPage />} />
        <Route exact path="/docs" element={<DocsPage />} />
        <Route path="/events/:id" element={<VisitorEventPage />} />
        <Route path="/cart" element={<CartPage />} />
        <Route path="/contact" element={<ContactPage />} />
        <Route exact path="/pricing" element={<PricingPage />} />

        <Route path="/login" element={<SignIn />} />
        <Route path="/signup" element={<SignUp />} />
        <Route path="/password-reset" element={<ForgotPassword />} />
        <Route path="/update-password" element={<UpdatePassword />} />

        <Route path="*" element={<NotFound />} />

        {/* Protected routes */}
        <Route path="/accept-invite" element={<ProtectedRoute><AcceptInvite /></ProtectedRoute>} />
        <Route path="/checkout" element={<ProtectedRoute><CheckoutPage /></ProtectedRoute>} />
        <Route path="/order/:id" element={<ProtectedRoute><OrderPage /></ProtectedRoute>} />
        <Route path="/orders" element={<ProtectedRoute><OrdersPage /></ProtectedRoute>} />
        <Route path="/payment/verify" element={<ProtectedRoute><PaymentConfirmationPage /></ProtectedRoute>} />
        <Route path="/ticket/:id" element={<ProtectedRoute><TicketPage /></ProtectedRoute>} />
        <Route path="/tickets" element={<ProtectedRoute><TicketsListPage /></ProtectedRoute>} />
      </Route>

      {/* Protected routes */}
      <Route element={<MainLayout />}>
        <Route path="/admin" element={<ProtectedRoute><HomePage /></ProtectedRoute>} />
        <Route path="/admin/scanner" element={<ProtectedRoute><ScannerPage /></ProtectedRoute>} />

        <Route path="/admin/events" element={<ProtectedRoute><EventsPage /></ProtectedRoute>} />
        <Route path="/admin/events/:id" element={<ProtectedRoute><EventPage /></ProtectedRoute>} />
        <Route path="/admin/events/:id/payouts" element={<PayoutsPage />} />

        <Route path="/admin/events/:id/tickets" element={<ProtectedRoute><EventTicketsLayout /></ProtectedRoute>}>
        <Route index element={<TicketsView />} />
          <Route path="types" element={<TicketTypesView />} />
        </Route>
        <Route path="/admin/bank-list" element={<ProtectedRoute><BankListPage /></ProtectedRoute>} />
        <Route path="/admin/events/:id/attendees" element={<ProtectedRoute><AttendeesPage /></ProtectedRoute>} />
        <Route path="/admin/settings" element={<ProtectedRoute><SettingsPage /></ProtectedRoute>} />
        <Route path="*" element={<ProtectedRoute><NotFound /></ProtectedRoute>} />
      </Route>
    </Routes>
  );
};

export default AppRoutes;