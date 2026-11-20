import React, { useState } from 'react';
import { motion } from 'motion/react';
import {
  ArrowLeft,
  ArrowRight,
  Calendar,
  Clock,
  MapPin,
  Share2,
  ShieldCheck,
  Sparkles,
  Train,
} from 'lucide-react';
import { formatINR } from '../../../lib/domain/rules';
import { SPRING_SPATIAL } from '../../../lib/motion/variants';
import { Button } from '../../../components/ui/Button';
import { TierBadge, ZoneComplianceMarker } from '../../../components/ui/TierBadge';
import { useEventDetail } from '../hooks/useEvents';

interface EventDetailViewProps {
  eventId: string;
  onBack: () => void;
  onSelectSeats: (eventId: string) => void;
  onNotify: (title: string, tone: 'success' | 'error' | 'info') => void;
}

export const EventDetailView: React.FC<EventDetailViewProps> = ({
  eventId,
  onBack,
  onSelectSeats,
  onNotify,
}) => {
  const { data: event, isLoading } = useEventDetail(eventId);
  const [saved, setSaved] = useState(false);

  if (isLoading || !event) {
    return (
      <div className="max-w-[1280px] mx-auto px-5 sm:px-10 py-10">
        <div className="h-[420px] rounded-[14px] bg-white border border-[#E7E5E0] animate-pulse" />
      </div>
    );
  }

  return (
    <div className="max-w-[1280px] mx-auto px-5 sm:px-10 pt-6 pb-16">
      {/* Top Breadcrumb & Back */}
      <div className="flex items-center justify-between gap-4 pb-6">
        <div className="flex items-center gap-3 text-[13px] text-[#5C6570] min-w-0">
          <button
            type="button"
            onClick={onBack}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[8px] bg-white border border-[#E7E5E0] text-[#0F1419] font-medium hover:bg-[#F4F3F0] cursor-pointer active:scale-95 transition-all shrink-0"
          >
            <ArrowLeft className="w-4 h-4" strokeWidth={1.75} />
            <span>Back</span>
          </button>
          <span className="hidden sm:inline truncate">
            Events / {event.category} / <span className="text-[#0F1419]">{event.title}</span>
          </span>
        </div>
      </div>

      {/* Two-Column Split: 7-col primary + 5-col sticky pricing */}
      <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 items-start">
        <div className="lg:col-span-7 space-y-8">
          <motion.div
            layoutId={`event-card-${event.id}`}
            transition={SPRING_SPATIAL}
            className="bg-white rounded-[14px] border border-[#E7E5E0] overflow-hidden shadow-card"
          >
            <motion.div
              layoutId={`event-image-${event.id}`}
              transition={SPRING_SPATIAL}
              className="relative aspect-[16/9] w-full bg-[#F4F3F0] overflow-hidden"
            >
              <img
                src={event.imageUrl}
                alt={event.title}
                referrerPolicy="no-referrer"
                className="w-full h-full object-cover"
              />
            </motion.div>

            <div className="p-6 sm:p-8 space-y-4">
              <div className="flex flex-wrap items-center gap-2 text-[13px] text-[#5C6570]">
                <span className="font-medium text-[#2D2A6E]">{event.category}</span>
                <span aria-hidden="true">·</span>
                <span>{event.venueName}</span>
                <span aria-hidden="true">·</span>
                <span className="num">{event.doorsOpenLabel}</span>
              </div>

              <motion.h1
                layoutId={`event-title-${event.id}`}
                transition={SPRING_SPATIAL}
                className="text-[28px] sm:text-[34px] font-bold leading-[1.15] text-[#0F1419]"
              >
                {event.title}
              </motion.h1>

              <p className="text-[16px] leading-[1.6] text-[#5C6570]">{event.subtitle}</p>

              <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 pt-4 border-t border-[#E7E5E0]">
                <div className="flex items-center gap-2.5 text-[13px]">
                  <Calendar className="w-4 h-4 text-[#2D2A6E]" strokeWidth={1.75} />
                  <span className="num font-medium text-[#0F1419]">{event.dateLabel}</span>
                </div>
                <div className="flex items-center gap-2.5 text-[13px]">
                  <Clock className="w-4 h-4 text-[#2D2A6E]" strokeWidth={1.75} />
                  <span className="num font-medium text-[#0F1419]">{event.timeLabel}</span>
                </div>
                <div className="flex items-center gap-2.5 text-[13px]">
                  <MapPin className="w-4 h-4 text-[#2D2A6E]" strokeWidth={1.75} />
                  <span className="font-medium text-[#0F1419]">{event.venueCity}</span>
                </div>
              </div>
            </div>
          </motion.div>

          {/* About the Experience */}
          <section className="bg-white rounded-[14px] border border-[#E7E5E0] p-6 sm:p-8 space-y-5">
            <h2 className="text-[22px] font-semibold text-[#0F1419]">About the Experience</h2>
            <div className="space-y-4 text-[15px] leading-[1.7] text-[#5C6570]">
              {event.aboutParagraphs.map((para, idx) => (
                <p key={idx}>{para}</p>
              ))}
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2">
              {event.highlights.map((item) => (
                <div
                  key={item.title}
                  className="p-4 rounded-[10px] bg-[#FAFAF7] border border-[#E7E5E0]"
                >
                  <div className="flex items-center gap-2 text-[14px] font-semibold text-[#0F1419]">
                    <Sparkles className="w-4 h-4 text-[#2D2A6E]" strokeWidth={1.75} />
                    <span>{item.title}</span>
                  </div>
                  <p className="text-[13px] text-[#5C6570] mt-1">{item.detail}</p>
                </div>
              ))}
            </div>
          </section>

          {/* Artists & Performers */}
          <section className="bg-white rounded-[14px] border border-[#E7E5E0] p-6 sm:p-8 space-y-5">
            <h2 className="text-[22px] font-semibold text-[#0F1419]">Artists &amp; Performers</h2>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
              {event.artists.map((artist) => (
                <div
                  key={artist.name}
                  className="p-4 rounded-[10px] bg-[#FAFAF7] border border-[#E7E5E0] space-y-1.5"
                >
                  <div className="text-[15px] font-semibold text-[#0F1419]">{artist.name}</div>
                  <div className="text-[12px] font-medium text-[#2D2A6E]">{artist.role}</div>
                  <p className="text-[13px] text-[#5C6570] leading-relaxed pt-1">{artist.bio}</p>
                </div>
              ))}
            </div>
          </section>

          {/* Venue & Arrival Info */}
          <section className="bg-white rounded-[14px] border border-[#E7E5E0] p-6 sm:p-8 space-y-4">
            <h2 className="text-[22px] font-semibold text-[#0F1419]">Venue &amp; Arrival Info</h2>
            <div className="space-y-3 text-[14px]">
              <div className="flex items-start gap-3">
                <MapPin className="w-4 h-4 text-[#2D2A6E] mt-1 shrink-0" strokeWidth={1.75} />
                <div>
                  <div className="font-medium text-[#0F1419]">{event.venueName}</div>
                  <div className="text-[#5C6570]">{event.venueAddress}</div>
                </div>
              </div>
              <div className="flex items-start gap-3">
                <Train className="w-4 h-4 text-[#0F766E] mt-1 shrink-0" strokeWidth={1.75} />
                <div className="text-[#5C6570]">{event.transitNote}</div>
              </div>
              <div className="flex items-start gap-3">
                <ShieldCheck className="w-4 h-4 text-[#B45309] mt-1 shrink-0" strokeWidth={1.75} />
                <div className="text-[#5C6570]">
                  Valid government photo ID required for 21+ Alcoholic VIP &amp; Couple zones.
                </div>
              </div>
            </div>
          </section>
        </div>

        {/* Sticky Right Pricing & Primary CTA Rail */}
        <aside className="lg:col-span-5 lg:sticky lg:top-24">
          <div className="bg-white rounded-[14px] border border-[#E7E5E0] shadow-card p-6 sm:p-7 space-y-6">
            <div>
              <div className="text-[12px] font-medium text-[#5C6570] uppercase tracking-wider">
                Ticket Tiers
              </div>
              <div className="mt-1 flex items-baseline gap-2">
                <span className="num text-[28px] font-semibold text-[#0F1419]">
                  {formatINR(event.startingPrice)}
                </span>
                <span className="text-[14px] text-[#5C6570]">to</span>
                <span className="num text-[22px] font-medium text-[#0F1419]">
                  {formatINR(event.maxPrice)}
                </span>
              </div>
              <p className="text-[13px] text-[#5C6570] mt-1">
                <span className="num font-medium text-[#0F1419]">
                  {event.totalSeats - event.bookedSeats}
                </span>{' '}
                seats remaining across {event.sections.length} zones
              </p>
            </div>

            <div className="divide-y divide-[#E7E5E0] border-y border-[#E7E5E0]">
              {event.sections.map((section) => (
                <div key={section.id} className="py-4 space-y-1.5">
                  <div className="flex items-center justify-between gap-2">
                    <TierBadge tier={section.tier} label={section.name} />
                    <span className="num text-[16px] font-semibold text-[#0F1419]">
                      {formatINR(section.price)}
                    </span>
                  </div>
                  <p className="text-[13px] text-[#5C6570]">{section.description}</p>
                  <div className="flex items-center justify-between pt-0.5">
                    <ZoneComplianceMarker
                      isAlcoholic={section.isAlcoholic}
                      minAge={section.minAge}
                    />
                    {section.fnbCredit > 0 && (
                      <span className="num text-[12px] font-medium text-[#0F766E]">
                        +{formatINR(section.fnbCredit)} F&amp;B credit
                      </span>
                    )}
                  </div>
                </div>
              ))}
            </div>

            {/* ONE Primary CTA */}
            <Button
              variant="primary"
              size="lg"
              className="w-full"
              onClick={() => onSelectSeats(event.id)}
            >
              <span>Select Seats &amp; Book</span>
              <ArrowRight className="w-4 h-4" strokeWidth={1.75} />
            </Button>

            <div className="flex items-center justify-between pt-1">
              <button
                type="button"
                onClick={() => {
                  if (typeof navigator !== 'undefined' && navigator.clipboard) {
                    navigator.clipboard.writeText(window.location.href).catch(() => {});
                  }
                  onNotify('Event link copied to clipboard.', 'info');
                }}
                className="inline-flex items-center gap-1.5 text-[13px] font-medium text-[#5C6570] hover:text-[#0F1419] cursor-pointer active:scale-95 transition-all"
              >
                <Share2 className="w-4 h-4" strokeWidth={1.75} />
                <span>Share event</span>
              </button>

              <button
                type="button"
                onClick={() => {
                  setSaved((prev) => !prev);
                  onNotify(saved ? 'Removed from saved' : 'Saved to your shortlist', 'info');
                }}
                className="text-[13px] font-medium text-[#5C6570] hover:text-[#0F1419] cursor-pointer active:scale-95 transition-all"
              >
                {saved ? 'Saved ✓' : 'Save for later'}
              </button>
            </div>
          </div>
        </aside>
      </div>
    </div>
  );
};
