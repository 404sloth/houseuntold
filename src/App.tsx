import { useCallback, useEffect, useState } from 'react';
import { AnimatePresence, LayoutGroup, motion } from 'motion/react';
import type { AttendeeDetailsForm, UserRole } from './lib/api/types';
import { PAGE_TRANSITION } from './lib/motion/variants';
import { TopNav, homeRouteForRole, type AppRoute } from './components/layout/TopNav';
import { Footer } from './components/layout/Footer';
import { ToastContainer, type ToastMessage } from './components/ui/ToastContainer';
import { useAuth } from './features/auth/hooks/useAuth';
import { LoginPage } from './features/auth/components/LoginPage';
import { AdminMfaPage } from './features/auth/components/AdminMfaPage';
import { AccountPage } from './features/auth/components/AccountPage';
import { ForbiddenPage } from './features/auth/components/ForbiddenPage';
import { RbacGuard } from './features/auth/components/RbacGuard';
import { LandingGateView } from './features/events/components/LandingGateView';
import { DiscoveryView } from './features/events/components/DiscoveryView';
import { EventDetailView } from './features/events/components/EventDetailView';
import { SeatMapView } from './features/booking/components/SeatMapView';
import { AttendeeDetailsView } from './features/booking/components/AttendeeDetailsView';
import { PaymentCheckoutView } from './features/booking/components/PaymentCheckoutView';
import { MyTicketsView } from './features/booking/components/MyTicketsView';
import { GateScannerView } from './features/scanner/components/GateScannerView';
import { BartenderPosView } from './features/counter/components/BartenderPosView';
import { AdminDashboardView } from './features/admin/components/AdminDashboardView';

function parsePathToRoute(pathname: string): AppRoute {
  if (pathname === '/login' || pathname === '/admin/login') return { name: 'login' };
  if (pathname === '/admin/login/mfa' || pathname === '/login/mfa') return { name: 'admin-mfa' };
  if (pathname === '/account') return { name: 'account' };
  if (pathname === '/tickets') return { name: 'tickets' };
  if (pathname === '/scan' || pathname.startsWith('/scan/')) return { name: 'scan' };
  if (pathname === '/counter' || pathname.startsWith('/counter/')) return { name: 'counter' };
  if (pathname === '/admin' || pathname.startsWith('/admin/')) return { name: 'admin' };
  if (pathname === '/403') return { name: 'forbidden' };
  if (pathname === '/events') return { name: 'events' };

  const seatMatch = pathname.match(/^\/events\/([^/]+)\/seats$/);
  if (seatMatch) return { name: 'seat-map', eventId: seatMatch[1] };
  const attMatch = pathname.match(/^\/events\/([^/]+)\/attendees$/);
  if (attMatch) return { name: 'attendees', eventId: attMatch[1] };
  const payMatch = pathname.match(/^\/events\/([^/]+)\/payment$/);
  if (payMatch) return { name: 'payment', eventId: payMatch[1] };
  const detailMatch = pathname.match(/^\/events\/([^/]+)$/);
  if (detailMatch) return { name: 'event-detail', eventId: detailMatch[1] };
  return { name: 'events' };
}

function routeToPath(route: AppRoute): string {
  switch (route.name) {
    case 'events':
      return '/';
    case 'event-detail':
      return `/events/${route.eventId}`;
    case 'seat-map':
      return `/events/${route.eventId}/seats`;
    case 'attendees':
      return `/events/${route.eventId}/attendees`;
    case 'payment':
      return `/events/${route.eventId}/payment`;
    case 'login':
      return '/login';
    case 'admin-mfa':
      return '/admin/login/mfa';
    case 'account':
      return '/account';
    case 'tickets':
      return '/tickets';
    case 'scan':
      return '/scan';
    case 'counter':
      return '/counter';
    case 'admin':
      return '/admin';
    case 'forbidden':
      return '/403';
  }
}

export default function App() {
  const { user, isLoading: isAuthLoading } = useAuth();
  const [route, setRoute] = useState<AppRoute>(() =>
    typeof window !== 'undefined' ? parsePathToRoute(window.location.pathname) : { name: 'events' }
  );
  const [toasts, setToasts] = useState<ToastMessage[]>([]);
  const [attendeeDraft, setAttendeeDraft] = useState<AttendeeDetailsForm | null>(null);
  const [mfaChallenge, setMfaChallenge] = useState<{ token: string; email: string } | null>(null);
  const [forbiddenRoles, setForbiddenRoles] = useState<UserRole[] | undefined>(undefined);
  const [postLoginRoute, setPostLoginRoute] = useState<AppRoute | null>(null);

  const notify = useCallback((title: string, tone: 'success' | 'error' | 'info') => {
    const id = `${Date.now()}_${Math.random().toString(36).slice(2, 7)}`;
    setToasts((prev) => [...prev, { id, title, tone }]);
    window.setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 4000);
  }, []);

  const dismissToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const navigate = useCallback((next: AppRoute) => {
    setRoute(next);
    if (typeof window !== 'undefined') {
      const targetPath = routeToPath(next);
      if (window.location.pathname !== targetPath) {
        window.history.pushState({}, '', targetPath);
      }
      window.scrollTo({ top: 0, behavior: 'smooth' });
    }
  }, []);

  useEffect(() => {
    const onPopState = () => {
      setRoute(parsePathToRoute(window.location.pathname));
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  // Strict unauthenticated gate: NO internal page or event detail opens without logging in.
  useEffect(() => {
    if (isAuthLoading) return;
    if (
      !user &&
      route.name !== 'events' &&
      route.name !== 'login' &&
      route.name !== 'admin-mfa'
    ) {
      if (
        route.name === 'account' ||
        route.name === 'tickets' ||
        route.name === 'event-detail' ||
        route.name === 'seat-map' ||
        route.name === 'attendees' ||
        route.name === 'payment'
      ) {
        setPostLoginRoute(route);
      }
      navigate({ name: 'login' });
      return;
    }

    // If non-patron staff/admin hits root '/', send them strictly to their role home
    if (user && user.role !== 'USER' && route.name === 'events') {
      navigate(homeRouteForRole(user));
    }
  }, [isAuthLoading, user, route, navigate]);

  const handleAuthenticatedForRole = useCallback(
    (role: UserRole) => {
      if (role === 'SCANNER') {
        setPostLoginRoute(null);
        navigate({ name: 'scan' });
        return;
      }
      if (role === 'BARTENDER') {
        setPostLoginRoute(null);
        navigate({ name: 'counter' });
        return;
      }
      if (role === 'ADMIN') {
        setPostLoginRoute(null);
        navigate({ name: 'admin' });
        return;
      }
      const next = postLoginRoute ?? { name: 'events' };
      setPostLoginRoute(null);
      navigate(next);
    },
    [navigate, postLoginRoute]
  );

  const handleRedirectForbidden = useCallback(
    (allowed: UserRole[]) => {
      setForbiddenRoles(allowed);
      navigate({ name: 'forbidden' });
    },
    [navigate]
  );

  return (
    <div className="min-h-screen flex flex-col bg-[#FAFAF7] text-[#0F1419]">
      <TopNav onNavigate={navigate} user={user} />

      <main className="flex-1">
        <LayoutGroup>
          <AnimatePresence mode="wait">
            {route.name === 'events' && (
              <motion.div
                key={user ? 'route-discovery' : 'route-landing-gate'}
                {...PAGE_TRANSITION}
              >
                {user ? (
                  user.role === 'USER' ? (
                    <DiscoveryView
                      onSelectEvent={(eventId) => navigate({ name: 'event-detail', eventId })}
                      onSelectSeats={(eventId) => navigate({ name: 'seat-map', eventId })}
                    />
                  ) : null
                ) : (
                  <LandingGateView onSignIn={() => navigate({ name: 'login' })} />
                )}
              </motion.div>
            )}

            {route.name === 'event-detail' && (
              <motion.div key={`route-detail-${route.eventId}`} {...PAGE_TRANSITION}>
                <RbacGuard
                  user={user}
                  isLoading={isAuthLoading}
                  allowedRoles={['USER']}
                  onRedirectLogin={() => navigate({ name: 'login' })}
                  onRedirectForbidden={handleRedirectForbidden}
                >
                  <EventDetailView
                    eventId={route.eventId}
                    onBack={() => navigate({ name: 'events' })}
                    onSelectSeats={(eventId) => navigate({ name: 'seat-map', eventId })}
                    onNotify={notify}
                  />
                </RbacGuard>
              </motion.div>
            )}

            {route.name === 'seat-map' && (
              <motion.div key={`route-seats-${route.eventId}`} {...PAGE_TRANSITION}>
                <RbacGuard
                  user={user}
                  isLoading={isAuthLoading}
                  allowedRoles={['USER', 'ADMIN']}
                  onRedirectLogin={() => navigate({ name: 'login' })}
                  onRedirectForbidden={handleRedirectForbidden}
                >
                  <SeatMapView
                    eventId={route.eventId}
                    onBack={() =>
                      navigate(
                        user?.role === 'ADMIN'
                          ? { name: 'admin' }
                          : { name: 'event-detail', eventId: route.eventId }
                      )
                    }
                    onContinue={(eventId) => navigate({ name: 'attendees', eventId })}
                    onNotify={notify}
                  />
                </RbacGuard>
              </motion.div>
            )}

            {route.name === 'attendees' && (
              <motion.div key={`route-attendees-${route.eventId}`} {...PAGE_TRANSITION}>
                <RbacGuard
                  user={user}
                  isLoading={isAuthLoading}
                  allowedRoles={['USER']}
                  onRedirectLogin={() => navigate({ name: 'login' })}
                  onRedirectForbidden={handleRedirectForbidden}
                >
                  <AttendeeDetailsView
                    eventId={route.eventId}
                    onBack={() => navigate({ name: 'seat-map', eventId: route.eventId })}
                    onNext={(form) => {
                      setAttendeeDraft(form);
                      navigate({ name: 'payment', eventId: route.eventId });
                    }}
                    onNotify={notify}
                  />
                </RbacGuard>
              </motion.div>
            )}

            {route.name === 'payment' && attendeeDraft && (
              <motion.div key={`route-payment-${route.eventId}`} {...PAGE_TRANSITION}>
                <RbacGuard
                  user={user}
                  isLoading={isAuthLoading}
                  allowedRoles={['USER']}
                  onRedirectLogin={() => navigate({ name: 'login' })}
                  onRedirectForbidden={handleRedirectForbidden}
                >
                  <PaymentCheckoutView
                    eventId={route.eventId}
                    attendeeForm={attendeeDraft}
                    onBack={() => navigate({ name: 'attendees', eventId: route.eventId })}
                    onCompleteToTickets={() => navigate({ name: 'tickets' })}
                    onNotify={notify}
                  />
                </RbacGuard>
              </motion.div>
            )}

            {route.name === 'login' && (
              <motion.div key="route-login" {...PAGE_TRANSITION}>
                <LoginPage
                  onBackHome={() => navigate({ name: 'events' })}
                  onAuthenticated={handleAuthenticatedForRole}
                  onRequireMfa={(token, email) => {
                    setMfaChallenge({ token, email });
                    navigate({ name: 'admin-mfa' });
                  }}
                  onNotify={notify}
                />
              </motion.div>
            )}

            {route.name === 'admin-mfa' && (
              <motion.div key="route-admin-mfa" {...PAGE_TRANSITION}>
                <AdminMfaPage
                  mfaChallengeToken={mfaChallenge?.token ?? null}
                  email={mfaChallenge?.email ?? ''}
                  onBackToLogin={() => navigate({ name: 'login' })}
                  onVerified={() => {
                    setMfaChallenge(null);
                    navigate({ name: 'admin' });
                  }}
                  onNotify={notify}
                />
              </motion.div>
            )}

            {route.name === 'account' && user && (
              <motion.div key="route-account" {...PAGE_TRANSITION}>
                <AccountPage
                  user={user}
                  onOpenTickets={() => navigate({ name: 'tickets' })}
                  onOpenWorkspace={() => handleAuthenticatedForRole(user.role)}
                  onSignedOut={() => navigate({ name: 'events' })}
                  onNotify={notify}
                />
              </motion.div>
            )}

            {route.name === 'tickets' && (
              <motion.div key="route-tickets" {...PAGE_TRANSITION}>
                <RbacGuard
                  user={user}
                  isLoading={isAuthLoading}
                  allowedRoles={['USER']}
                  onRedirectLogin={() => navigate({ name: 'login' })}
                  onRedirectForbidden={handleRedirectForbidden}
                >
                  <MyTicketsView onBrowseEvents={() => navigate({ name: 'events' })} />
                </RbacGuard>
              </motion.div>
            )}

            {route.name === 'scan' && (
              <motion.div key="route-scan" {...PAGE_TRANSITION}>
                <RbacGuard
                  user={user}
                  isLoading={isAuthLoading}
                  allowedRoles={['ADMIN', 'SCANNER']}
                  onRedirectLogin={() => navigate({ name: 'login' })}
                  onRedirectForbidden={handleRedirectForbidden}
                >
                  <GateScannerView onNotify={notify} />
                </RbacGuard>
              </motion.div>
            )}

            {route.name === 'counter' && (
              <motion.div key="route-counter" {...PAGE_TRANSITION}>
                <RbacGuard
                  user={user}
                  isLoading={isAuthLoading}
                  allowedRoles={['ADMIN', 'BARTENDER']}
                  onRedirectLogin={() => navigate({ name: 'login' })}
                  onRedirectForbidden={handleRedirectForbidden}
                >
                  <BartenderPosView onNotify={notify} />
                </RbacGuard>
              </motion.div>
            )}

            {route.name === 'admin' && (
              <motion.div key="route-admin" {...PAGE_TRANSITION}>
                <RbacGuard
                  user={user}
                  isLoading={isAuthLoading}
                  allowedRoles={['ADMIN']}
                  onRedirectLogin={() => navigate({ name: 'login' })}
                  onRedirectForbidden={handleRedirectForbidden}
                >
                  <AdminDashboardView
                    onInspectSeatMap={(eventId) => navigate({ name: 'seat-map', eventId })}
                    onNotify={notify}
                  />
                </RbacGuard>
              </motion.div>
            )}

            {route.name === 'forbidden' && (
              <motion.div key="route-forbidden" {...PAGE_TRANSITION}>
                <ForbiddenPage
                  currentRole={user?.role}
                  requiredRoles={forbiddenRoles}
                  onReturnHome={() => navigate(homeRouteForRole(user))}
                />
              </motion.div>
            )}
          </AnimatePresence>
        </LayoutGroup>
      </main>

      {route.name !== 'seat-map' && <Footer onNavigate={navigate} user={user} />}

      <ToastContainer toasts={toasts} onDismiss={dismissToast} />
    </div>
  );
}
