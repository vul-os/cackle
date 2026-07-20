import React from 'react';
import { Routes, Route } from 'react-router-dom';

// Layouts
import BlankLayout from './components/layout/blank-layout';
import MainLayout from './components/layout/main-layout';
import ProtectedRoute from './components/auth/protected-route';

// Public / visitor pages
import LandingPage from './pages/visitor/landing';
import DocsPage from './pages/visitor/docs';
import ContactPage from './pages/visitor/contact';
import VisitorEventPage from './pages/visitor/events/event';
import BrowsePage from './pages/visitor/browse';
import CartPage from './pages/visitor/cart';
import CheckoutPage from './pages/visitor/checkout';
import PaymentConfirmationPage from './pages/visitor/payment/confirmation';
import OrdersPage from './pages/visitor/orders';
import OrderPage from './pages/visitor/orders/order';
import TicketPage from './pages/visitor/ticket';
import TicketsListPage from './pages/visitor/tickets';
import NotFound from './pages/not-found';

// Auth pages
import SignIn from './pages/auth/signin';
import SignUp from './pages/auth/signup';
import ForgotPassword from './pages/auth/forgot-password';
import UpdatePassword from './pages/auth/update-password';

// Organizer console
import HomePage from './pages/organizers/home';
import EventsPage from './pages/organizers/events';
import EventPage from './pages/organizers/events/event';
import EventStatsPage from './pages/organizers/events/event/stats';
import EventAttendeesPage from './pages/organizers/events/event/attendees';
import EventTicketTypesPage from './pages/organizers/events/event/tickets';
import ScannerPage from './pages/organizers/scanner';
import SettingsPage from './pages/organizers/settings';
import PricingPage from './pages/organizers/pricing';

const AppRoutes = () => {
    return (
        <Routes>
            {/* Public / visitor surface — pages bring their own header/footer */}
            <Route element={<BlankLayout />}>
                <Route path="/" element={<LandingPage />} />
                <Route path="/docs" element={<DocsPage />} />
                <Route path="/events" element={<BrowsePage />} />
                <Route path="/events/:slug" element={<VisitorEventPage />} />
                <Route path="/cart" element={<CartPage />} />
                <Route path="/contact" element={<ContactPage />} />
                <Route path="/pricing" element={<PricingPage />} />

                <Route path="/login" element={<SignIn />} />
                <Route path="/signup" element={<SignUp />} />
                <Route path="/password-reset" element={<ForgotPassword />} />
                <Route path="/update-password" element={<UpdatePassword />} />

                {/* Protected visitor routes */}
                <Route
                    path="/checkout/:eventId"
                    element={
                        <ProtectedRoute>
                            <CheckoutPage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/order/:id"
                    element={
                        <ProtectedRoute>
                            <OrderPage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/orders"
                    element={
                        <ProtectedRoute>
                            <OrdersPage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/payment/verify"
                    element={
                        <ProtectedRoute>
                            <PaymentConfirmationPage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/ticket/:id"
                    element={
                        <ProtectedRoute>
                            <TicketPage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/tickets"
                    element={
                        <ProtectedRoute>
                            <TicketsListPage />
                        </ProtectedRoute>
                    }
                />

                <Route path="*" element={<NotFound />} />
            </Route>

            {/* Organizer console */}
            <Route element={<MainLayout />}>
                <Route
                    path="/admin"
                    element={
                        <ProtectedRoute>
                            <HomePage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/admin/scanner"
                    element={
                        <ProtectedRoute>
                            <ScannerPage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/admin/events"
                    element={
                        <ProtectedRoute>
                            <EventsPage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/admin/events/:id"
                    element={
                        <ProtectedRoute>
                            <EventPage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/admin/events/:id/stats"
                    element={
                        <ProtectedRoute>
                            <EventStatsPage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/admin/events/:id/attendees"
                    element={
                        <ProtectedRoute>
                            <EventAttendeesPage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/admin/events/:id/tickets"
                    element={
                        <ProtectedRoute>
                            <EventTicketTypesPage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/admin/settings"
                    element={
                        <ProtectedRoute>
                            <SettingsPage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="*"
                    element={
                        <ProtectedRoute>
                            <NotFound />
                        </ProtectedRoute>
                    }
                />
            </Route>
        </Routes>
    );
};

export default AppRoutes;
