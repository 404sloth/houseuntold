import React from 'react';
import { motion } from 'motion/react';
import { ArrowRight, Calendar, MapPin, Sparkles } from 'lucide-react';
import type { VenuEvent } from '../../../lib/api/types';
import { formatINR } from '../../../lib/domain/rules';
import { SPRING_SPATIAL, STAGGER_ITEM } from '../../../lib/motion/variants';
import { TierBadge } from '../../../components/ui/TierBadge';

interface EventCardProps {
  event: VenuEvent;
  onSelect: (eventId: string) => void;
}

export const EventCard: React.FC<EventCardProps> = ({ event, onSelect }) => {
  const occupancyPct = Math.min(
    100,
    Math.round((event.bookedSeats / Math.max(1, event.totalSeats)) * 100)
  );
  const seatsLeft = Math.max(0, event.totalSeats - event.bookedSeats);

  return (
    <motion.article
      variants={STAGGER_ITEM}
      layoutId={`event-card-${event.id}`}
      transition={SPRING_SPATIAL}
      className="group bg-white rounded-[14px] border border-[#E7E5E0] shadow-card hover:shadow-editorial transition-all duration-300 overflow-hidden flex flex-col"
    >
      <button
        type="button"
        onClick={() => onSelect(event.id)}
        className="text-left flex-1 flex flex-col cursor-pointer focus-visible:outline-2 focus-visible:outline-[#2D2A6E]"
      >
        <motion.div
          layoutId={`event-image-${event.id}`}
          transition={SPRING_SPATIAL}
          className="relative aspect-[16/10] w-full bg-[#F4F3F0] overflow-hidden"
        >
          <img
            src={event.imageUrl}
            alt={event.title}
            referrerPolicy="no-referrer"
            className="w-full h-full object-cover group-hover:scale-[1.03] transition-transform duration-500"
          />
          <div className="absolute inset-0 bg-gradient-to-t from-[#0F1419]/50 via-[#0F1419]/10 to-transparent" />

          {/* Crisp Top Badges */}
          <div className="absolute top-3 left-3 right-3 flex items-center justify-between gap-2">
            <span className="px-2.5 py-1 rounded-[6px] bg-white/95 backdrop-blur-xs text-[#0F1419] text-[11px] font-semibold tracking-wide uppercase">
              {event.layoutType === 'CLUB' ? 'Club Floor' : 'Seated Hall'}
            </span>
            {event.status === 'ALMOST_FULL' && (
              <span className="num px-2.5 py-1 rounded-[6px] bg-[#FEF3C7] text-[#B45309] text-[11px] font-semibold">
                {seatsLeft} Left
              </span>
            )}
          </div>

          <div className="absolute bottom-3 left-3 right-3 flex items-center justify-between gap-2">
            <span className="px-2.5 py-1 rounded-[6px] bg-white/95 text-[#0F1419] text-[12px] font-medium">
              {event.category}
            </span>
            {event.fnbCreditPerVip > 0 && (
              <span className="num inline-flex items-center gap-1 px-2.5 py-1 rounded-[6px] bg-[#CCFBF1]/95 text-[#0F766E] text-[11px] font-semibold">
                <Sparkles className="w-3 h-3" strokeWidth={1.75} />
                {formatINR(event.fnbCreditPerVip)} Bar Credit
              </span>
            )}
          </div>
        </motion.div>

        <div className="p-6 flex-1 flex flex-col justify-between gap-5">
          <div className="space-y-3">
            <motion.h3
              layoutId={`event-title-${event.id}`}
              transition={SPRING_SPATIAL}
              className="text-[19px] font-semibold leading-[1.3] text-[#0F1419] group-hover:text-[#2D2A6E] transition-colors"
            >
              {event.title}
            </motion.h3>

            <p className="text-[13px] text-[#5C6570] line-clamp-2 leading-relaxed">
              {event.subtitle}
            </p>

            <div className="space-y-1.5 text-[13px] text-[#5C6570]">
              <div className="flex items-center gap-2">
                <Calendar className="w-4 h-4 shrink-0 text-[#5C6570]" strokeWidth={1.75} />
                <span className="num text-[12.5px]">
                  {event.dateLabel} · {event.timeLabel}
                </span>
              </div>
              <div className="flex items-center gap-2">
                <MapPin className="w-4 h-4 shrink-0 text-[#5C6570]" strokeWidth={1.75} />
                <span>
                  {event.venueName}, {event.venueCity}
                </span>
              </div>
            </div>

            <div className="flex flex-wrap items-center gap-1.5 pt-0.5">
              {event.sections.map((section) => (
                <TierBadge
                  key={section.id}
                  tier={section.tier}
                  label={section.name}
                  isAlcoholic={section.isAlcoholic}
                />
              ))}
            </div>
          </div>

          <div className="space-y-3 pt-4 border-t border-[#E7E5E0]">
            {/* Minimal Occupancy Meter */}
            <div className="space-y-1.5">
              <div className="flex items-center justify-between text-[11px] text-[#5C6570]">
                <span>Floor Availability</span>
                <span className="num font-medium text-[#0F1419]">
                  {seatsLeft} / {event.totalSeats} seats open
                </span>
              </div>
              <div className="h-1.5 w-full rounded-full bg-[#F4F3F0] overflow-hidden">
                <div
                  className="h-full rounded-full bg-[#2D2A6E] transition-all duration-500"
                  style={{ width: `${occupancyPct}%` }}
                />
              </div>
            </div>

            <div className="flex items-center justify-between gap-4 pt-1">
              <div>
                <span className="block text-[11px] font-medium text-[#5C6570] uppercase tracking-wider">
                  From
                </span>
                <span className="num text-[18px] font-semibold text-[#0F1419]">
                  {formatINR(event.startingPrice)}
                </span>
              </div>
              <span className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-[8px] bg-[#FAFAF7] border border-[#E7E5E0] text-[13px] font-semibold text-[#2D2A6E] group-hover:bg-[#2D2A6E] group-hover:text-white group-hover:border-[#2D2A6E] transition-all">
                <span>Select Seats</span>
                <ArrowRight className="w-3.5 h-3.5" strokeWidth={1.75} />
              </span>
            </div>
          </div>
        </div>
      </button>
    </motion.article>
  );
};
