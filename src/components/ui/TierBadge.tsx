import React from 'react';
import type { TierType } from '../../lib/api/types';

interface TierBadgeProps {
  tier: TierType;
  label?: string;
  isAlcoholic?: boolean;
}

const TIER_STYLES: Record<TierType, { bg: string; text: string; border: string; defaultLabel: string }> = {
  VIP: {
    bg: 'bg-[#FEF3C7]',
    text: 'text-[#B45309]',
    border: 'border-[#B45309]/20',
    defaultLabel: 'VIP / Stage',
  },
  COUPLE: {
    bg: 'bg-[#FCE7F0]',
    text: 'text-[#E11D63]',
    border: 'border-[#E11D63]/20',
    defaultLabel: 'Couple',
  },
  GENERAL: {
    bg: 'bg-[#CCFBF1]',
    text: 'text-[#0F766E]',
    border: 'border-[#0F766E]/20',
    defaultLabel: 'General',
  },
};

export const TierBadge: React.FC<TierBadgeProps> = ({ tier, label, isAlcoholic }) => {
  const s = TIER_STYLES[tier];
  return (
    <span
      className={`inline-flex items-center gap-1.5 h-7 px-2.5 rounded-[6px] border text-[12px] font-medium num whitespace-nowrap shrink-0 ${s.bg} ${s.text} ${s.border}`}
    >
      {typeof isAlcoholic === 'boolean' && (
        <span
          aria-hidden="true"
          className={
            isAlcoholic
              ? 'w-2 h-2 rounded-full bg-[#B45309] shrink-0'
              : 'w-2 h-2 rounded-full border-[1.75px] border-[#0F766E] bg-transparent shrink-0'
          }
        />
      )}
      <span>{label ?? s.defaultLabel}</span>
    </span>
  );
};

interface ZoneComplianceMarkerProps {
  isAlcoholic: boolean;
  minAge?: number;
}

export const ZoneComplianceMarker: React.FC<ZoneComplianceMarkerProps> = ({ isAlcoholic, minAge }) => {
  return (
    <span className="inline-flex items-center gap-1.5 text-[12px] text-[#5C6570] whitespace-nowrap">
      <span
        aria-hidden="true"
        className={
          isAlcoholic
            ? 'w-2 h-2 rounded-full bg-[#B45309] shrink-0'
            : 'w-2 h-2 rounded-full border-[1.75px] border-[#0F766E] bg-transparent shrink-0'
        }
      />
      <span>{isAlcoholic ? `Alcoholic Zone (${minAge ?? 21}+)` : 'Non-Alcoholic (All Ages)'}</span>
    </span>
  );
};
