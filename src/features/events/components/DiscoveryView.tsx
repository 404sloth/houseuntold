import React, { useMemo, useState } from 'react';
import { AnimatePresence, motion } from 'motion/react';
import {
  ArrowRight,
  Award,
  Calendar,
  MapPin,
  Sparkles,
  Users,
  X,
} from 'lucide-react';
import type { TierType, VenuEvent } from '../../../lib/api/types';
import { formatINR } from '../../../lib/domain/rules';
import { SPRING_SPATIAL, STAGGER_CONTAINER } from '../../../lib/motion/variants';
import { Button } from '../../../components/ui/Button';
import { TierBadge, ZoneComplianceMarker } from '../../../components/ui/TierBadge';
import { useEvents } from '../hooks/useEvents';
import { EventCard } from './EventCard';

interface DiscoveryViewProps {
  onSelectEvent: (eventId: string) => void;
  onSelectSeats: (eventId: string) => void;
}

const CATEGORIES = [
  'All Shows',
  'Live Music',
  'Stand-up Comedy',
  'Theater & Plays',
  'Lounge Sessions',
] as const;

const TIER_FILTERS: Array<{ id: 'ALL' | TierType; label: string }> = [
  { id: 'ALL', label: 'All Zones' },
  { id: 'VIP', label: 'VIP' },
  { id: 'COUPLE', label: 'Couple' },
  { id: 'GENERAL', label: 'General' },
];

export const DiscoveryView: React.FC<DiscoveryViewProps> = ({
  onSelectEvent,
  onSelectSeats,
}) => {
  const { data: events = [], isLoading, isError, refetch } = useEvents();
  const [selectedCategory, setSelectedCategory] = useState<string>('All Shows');
  const [selectedTier, setSelectedTier] = useState<'ALL' | TierType>('ALL');
  const [spotlightEvent, setSpotlightEvent] = useState<VenuEvent | null>(null);

  const visibleEvents = useMemo(() => {
    return events.filter((evt) => {
      if (evt.status === 'DRAFT' || evt.status === 'PAUSED') return false;
      const matchCat = selectedCategory === 'All Shows' || evt.category === selectedCategory;
      const matchTier =
        selectedTier === 'ALL' || evt.sections.some((sec) => sec.tier === selectedTier);
      return matchCat && matchTier;
    });
  }, [events, selectedCategory, selectedTier]);

  return (
    <div className="max-w-[1280px] mx-auto px-5 sm:px-10 pt-8 sm:pt-12 pb-16 space-y-8">
      {/* Clean Patron Events Catalog Header */}
      <section className="flex flex-col md:flex-row md:items-end justify-between gap-4 pb-6 border-b border-[#E7E5E0]">
        <div>
          <p className="text-[12px] font-medium tracking-[0.06em] uppercase text-[#5C6570]">
            Season 2026 · Curated Live Lineup
          </p>
          <h1 className="text-[32px] sm:text-[38px] font-bold tracking-tight text-[#0F1419] mt-1">
            Upcoming Events
          </h1>
          <p className="text-[14px] text-[#5C6570] mt-1">
            Select any show to preview highlights, tier perks, and book exact floor seats.
          </p>
        </div>

        <div className="flex items-center gap-1 p-1 bg-white border border-[#E7E5E0] rounded-[10px] self-start">
          {TIER_FILTERS.map((tier) => (
            <button
              key={tier.id}
              type="button"
              onClick={() => setSelectedTier(tier.id)}
              className={`px-3 py-1.5 text-[12px] font-medium rounded-[7px] whitespace-nowrap cursor-pointer active:scale-95 transition-all ${
                selectedTier === tier.id
                  ? 'bg-[#2D2A6E] text-white'
                  : 'text-[#5C6570] hover:text-[#0F1419]'
              }`}
            >
              {tier.label}
            </button>
          ))}
        </div>
      </section>

      {/* Category Filter Tabs */}
      <div className="flex items-center gap-2 overflow-x-auto pb-1">
        {CATEGORIES.map((category) => {
          const active = selectedCategory === category;
          return (
            <button
              key={category}
              type="button"
              onClick={() => setSelectedCategory(category)}
              className={`px-4 py-2 rounded-[8px] text-[13px] font-medium whitespace-nowrap shrink-0 border cursor-pointer active:scale-95 transition-all ${
                active
                  ? 'bg-[#0F1419] text-white border-[#0F1419]'
                  : 'bg-white text-[#5C6570] border-[#E7E5E0] hover:text-[#0F1419]'
              }`}
            >
              {category}
            </button>
          );
        })}
      </div>

      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {[1, 2, 3].map((n) => (
            <div
              key={n}
              className="h-[410px] rounded-[14px] bg-white border border-[#E7E5E0] animate-pulse"
            />
          ))}
        </div>
      ) : isError ? (
        <div className="rounded-[14px] bg-white border border-[#E7E5E0] p-8 text-center space-y-3">
          <p className="text-[15px] text-[#0F1419] font-medium">
            Could not load upcoming events.
          </p>
          <Button variant="secondary" onClick={() => refetch()}>
            Retry
          </Button>
        </div>
      ) : visibleEvents.length === 0 ? (
        <div className="rounded-[14px] bg-white border border-[#E7E5E0] p-10 text-center space-y-3">
          <p className="text-[16px] font-medium text-[#0F1419]">
            No shows match this category and zone filter.
          </p>
          <Button
            variant="secondary"
            onClick={() => {
              setSelectedCategory('All Shows');
              setSelectedTier('ALL');
            }}
          >
            Reset filters
          </Button>
        </div>
      ) : (
        <motion.div
          variants={STAGGER_CONTAINER}
          initial="hidden"
          animate="show"
          className={`grid grid-cols-1 md:grid-cols-2 gap-6 ${
            visibleEvents.length === 4 ? '' : 'lg:grid-cols-3'
          }`}
        >
          {visibleEvents.map((evt) => (
            <EventCard
              key={evt.id}
              event={evt}
              onSelect={() => setSpotlightEvent(evt)}
            />
          ))}
        </motion.div>
      )}

      {/* Origin-Aware Event Spotlight Modal */}
      <AnimatePresence>
        {spotlightEvent && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-[#0F1419]/40 backdrop-blur-[2px]"
            role="dialog"
            aria-modal="true"
            aria-labelledby="spotlight-event-title"
            onClick={() => setSpotlightEvent(null)}
          >
            <motion.div
              layoutId={`event-card-${spotlightEvent.id}`}
              initial={{ opacity: 0, scale: 0.94 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.94 }}
              transition={SPRING_SPATIAL}
              onClick={(e) => e.stopPropagation()}
              className="w-full max-w-2xl bg-white rounded-[18px] border border-[#E7E5E0] shadow-modal overflow-hidden max-h-[90vh] flex flex-col"
            >
              <div className="relative aspect-[16/8] w-full bg-[#F4F3F0] shrink-0 overflow-hidden">
                <img
                  src={spotlightEvent.imageUrl}
                  alt={spotlightEvent.title}
                  referrerPolicy="no-referrer"
                  className="w-full h-full object-cover"
                />
                <div className="absolute inset-0 bg-gradient-to-t from-[#0F1419]/60 via-[#0F1419]/15 to-transparent" />

                <button
                  type="button"
                  onClick={() => setSpotlightEvent(null)}
                  aria-label="Close event preview"
                  className="absolute top-3.5 right-3.5 w-9 h-9 rounded-[8px] bg-white/95 border border-[#E7E5E0] flex items-center justify-center text-[#0F1419] hover:bg-white cursor-pointer active:scale-95 transition-all"
                >
                  <X className="w-4 h-4" strokeWidth={1.75} />
                </button>

                <div className="absolute bottom-3.5 left-5 right-5 flex flex-wrap items-center justify-between gap-2">
                  <span className="px-2.5 py-1 rounded-[6px] bg-white/95 text-[#0F1419] text-[12px] font-semibold">
                    {spotlightEvent.category}
                  </span>
                  <span className="num px-2.5 py-1 rounded-[6px] bg-[#FEF3C7] text-[#B45309] text-[11px] font-semibold inline-flex items-center gap-1">
                    <Users className="w-3.5 h-3.5" strokeWidth={1.75} />
                    {Math.max(0, spotlightEvent.totalSeats - spotlightEvent.bookedSeats)} seats
                    remaining
                  </span>
                </div>
              </div>

              <div className="p-6 sm:p-7 overflow-y-auto space-y-5">
                <div className="space-y-2">
                  <div className="flex flex-wrap items-center gap-3 text-[12.5px] text-[#5C6570]">
                    <span className="inline-flex items-center gap-1.5 num">
                      <Calendar className="w-3.5 h-3.5 text-[#2D2A6E]" strokeWidth={1.75} />
                      {spotlightEvent.dateLabel} · {spotlightEvent.timeLabel}
                    </span>
                    <span aria-hidden="true">·</span>
                    <span className="inline-flex items-center gap-1.5">
                      <MapPin className="w-3.5 h-3.5 text-[#2D2A6E]" strokeWidth={1.75} />
                      {spotlightEvent.venueName}, {spotlightEvent.venueCity}
                    </span>
                  </div>

                  <h2
                    id="spotlight-event-title"
                    className="text-[24px] sm:text-[26px] font-bold leading-tight text-[#0F1419]"
                  >
                    {spotlightEvent.title}
                  </h2>
                  <p className="text-[14px] text-[#5C6570] leading-relaxed">
                    {spotlightEvent.subtitle}
                  </p>
                </div>

                {/* Achievements / Why Book Highlights */}
                <div className="space-y-2.5">
                  <div className="inline-flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-[#2D2A6E] num">
                    <Award className="w-3.5 h-3.5" strokeWidth={1.75} />
                    <span>Show Highlights &amp; Guest Perks</span>
                  </div>
                  <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                    {spotlightEvent.highlights.slice(0, 3).map((hl) => (
                      <div
                        key={hl.title}
                        className="p-3.5 rounded-[10px] bg-[#FAFAF7] border border-[#E7E5E0] space-y-1"
                      >
                        <div className="flex items-center gap-1.5 text-[13px] font-semibold text-[#0F1419]">
                          <Sparkles className="w-3.5 h-3.5 text-[#B45309]" strokeWidth={1.75} />
                          <span>{hl.title}</span>
                        </div>
                        <p className="text-[12px] text-[#5C6570] leading-snug">{hl.detail}</p>
                      </div>
                    ))}
                  </div>
                </div>

                {/* Zone Tiers */}
                <div className="divide-y divide-[#E7E5E0] border-y border-[#E7E5E0]">
                  {spotlightEvent.sections.map((sec) => (
                    <div key={sec.id} className="py-3 flex items-center justify-between gap-3">
                      <div className="space-y-0.5">
                        <div className="flex items-center gap-2">
                          <TierBadge tier={sec.tier} label={sec.name} />
                          <ZoneComplianceMarker
                            isAlcoholic={sec.isAlcoholic}
                            minAge={sec.minAge}
                          />
                        </div>
                        <div className="text-[12px] text-[#5C6570]">{sec.description}</div>
                      </div>
                      <div className="text-right shrink-0">
                        <div className="num text-[15px] font-semibold text-[#0F1419]">
                          {formatINR(sec.price)}
                        </div>
                        {sec.fnbCredit > 0 && (
                          <div className="num text-[11px] font-medium text-[#0F766E]">
                            +{formatINR(sec.fnbCredit)} Bar Credit
                          </div>
                        )}
                      </div>
                    </div>
                  ))}
                </div>

                {/* Modal Actions: 1 Primary CTA + Secondary Brief Link */}
                <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 pt-1">
                  <Button
                    variant="secondary"
                    size="md"
                    onClick={() => {
                      const targetId = spotlightEvent.id;
                      setSpotlightEvent(null);
                      onSelectEvent(targetId);
                    }}
                  >
                    <span>View Full Event Brief</span>
                  </Button>

                  <Button
                    variant="primary"
                    size="lg"
                    onClick={() => {
                      const targetId = spotlightEvent.id;
                      setSpotlightEvent(null);
                      onSelectSeats(targetId);
                    }}
                  >
                    <span>Select Seats &amp; Book</span>
                    <ArrowRight className="w-4 h-4" strokeWidth={1.75} />
                  </Button>
                </div>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>
    </div>
  );
};
