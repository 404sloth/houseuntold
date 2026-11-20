import React from 'react';
import { Calendar, LogIn, MapPin, Scissors, ShieldCheck, Ticket as TicketIcon, User as UserIcon } from 'lucide-react';
import type { User } from '../../lib/api/types';
import { VenuLogo } from '../ui/VenuLogo';
import { Button } from '../ui/Button';

export type AppRoute =
  | { name: 'events' }
  | { name: 'event-detail'; eventId: string }
  | { name: 'seat-map'; eventId: string }
  | { name: 'attendees'; eventId: string }
  | { name: 'payment'; eventId: string }
  | { name: 'login' }
  | { name: 'admin-mfa' }
  | { name: 'account' }
  | { name: 'tickets' }
  | { name: 'scan' }
  | { name: 'counter' }
  | { name: 'admin' }
  | { name: 'forbidden' };

interface TopNavProps {
  onNavigate: (route: AppRoute) => void;
  user: User | null;
}

export function homeRouteForRole(user: User | null): AppRoute {
  if (!user) return { name: 'events' };
  if (user.role === 'SCANNER') return { name: 'scan' };
  if (user.role === 'BARTENDER') return { name: 'counter' };
  if (user.role === 'ADMIN') return { name: 'admin' };
  return { name: 'events' };
}

export const TopNav: React.FC<TopNavProps> = ({ onNavigate, user }) => {
  const logoDestination = homeRouteForRole(user);

  const navLinks = (): Array<{ label: string; route: AppRoute; icon?: React.ReactNode }> => {
    if (!user) return [];
    if (user.role === 'USER') {
      return [
        {
          label: 'Events',
          route: { name: 'events' },
          icon: <Calendar className="w-4 h-4" strokeWidth={1.75} />,
        },
        {
          label: 'My Tickets',
          route: { name: 'tickets' },
          icon: <TicketIcon className="w-4 h-4" strokeWidth={1.75} />,
        },
      ];
    }
    if (user.role === 'SCANNER') {
      return [
        {
          label: 'Gate Scanner',
          route: { name: 'scan' },
          icon: <Scissors className="w-4 h-4" strokeWidth={1.75} />,
        },
      ];
    }
    if (user.role === 'BARTENDER') {
      return [
        {
          label: 'Bar Counter',
          route: { name: 'counter' },
          icon: <MapPin className="w-4 h-4" strokeWidth={1.75} />,
        },
      ];
    }
    if (user.role === 'ADMIN') {
      return [
        {
          label: 'Admin Console',
          route: { name: 'admin' },
          icon: <ShieldCheck className="w-4 h-4" strokeWidth={1.75} />,
        },
      ];
    }
    return [];
  };

  const links = navLinks();

  return (
    <header className="sticky top-0 z-40 bg-[#FAFAF7]/95 backdrop-blur-sm border-b border-[#E7E5E0]">
      <div className="max-w-[1280px] mx-auto px-5 sm:px-10 h-16 flex items-center justify-between gap-6">
        <VenuLogo onClick={() => onNavigate(logoDestination)} />

        <div className="flex items-center gap-2 sm:gap-3 shrink-0">
          {links.map((item) => (
            <button
              key={item.label}
              type="button"
              onClick={() => onNavigate(item.route)}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[8px] text-[13px] font-medium text-[#5C6570] hover:text-[#0F1419] hover:bg-white cursor-pointer active:scale-95 transition-all"
            >
              {item.icon}
              <span>{item.label}</span>
            </button>
          ))}

          {user ? (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => onNavigate({ name: 'account' })}
            >
              <UserIcon className="w-4 h-4 text-[#2D2A6E]" strokeWidth={1.75} />
              <span className="max-w-[130px] truncate">{user.name.split(' ')[0]}</span>
            </Button>
          ) : (
            <Button
              variant="primary"
              size="sm"
              onClick={() => onNavigate({ name: 'login' })}
            >
              <LogIn className="w-4 h-4" strokeWidth={1.75} />
              <span>Sign In</span>
            </Button>
          )}
        </div>
      </div>
    </header>
  );
};
