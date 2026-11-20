import React from 'react';
import type { User } from '../../lib/api/types';
import { VenuLogo } from '../ui/VenuLogo';
import { homeRouteForRole, type AppRoute } from './TopNav';

interface FooterProps {
  onNavigate: (route: AppRoute) => void;
  user: User | null;
}

export const Footer: React.FC<FooterProps> = ({ onNavigate, user }) => {
  return (
    <footer className="border-t border-[#E7E5E0] bg-[#FAFAF7] mt-16">
      <div className="max-w-[1280px] mx-auto px-5 sm:px-10 py-8 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div className="flex items-center gap-4">
          <VenuLogo onClick={() => onNavigate(homeRouteForRole(user))} />
          <span className="text-[13px] text-[#5C6570]">
            © 2026 Venu Live Ticketing. All rights reserved.
          </span>
        </div>
        <div className="text-[13px] text-[#5C6570]">
          Mumbai · Bengaluru · Intimate Venues &amp; Listening Rooms
        </div>
      </div>
    </footer>
  );
};
