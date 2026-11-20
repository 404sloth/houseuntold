import React, { useEffect, useMemo, useState } from 'react';
import { ArrowLeft, ArrowRight, Clock, Minus, Plus, RotateCcw, X } from 'lucide-react';
import type { Seat, TierType } from '../../../lib/api/types';
import { formatINR, formatTimerClock } from '../../../lib/domain/rules';
import { Button } from '../../../components/ui/Button';
import { ZoneComplianceMarker } from '../../../components/ui/TierBadge';
import { useSeatSelection } from '../hooks/useSeatSelection';

interface SeatMapViewProps {
  eventId: string;
  onBack: () => void;
  onContinue: (eventId: string) => void;
  onNotify: (title: string, tone: 'success' | 'error' | 'info') => void;
}

const TIER_PILL_COLOR: Record<TierType, { base: string; border: string; text: string }> = {
  VIP: { base: 'bg-[#FEF3C7]', border: 'border-[#B45309]/40', text: 'text-[#B45309]' },
  COUPLE: { base: 'bg-[#FCE7F0]', border: 'border-[#E11D63]/40', text: 'text-[#E11D63]' },
  GENERAL: { base: 'bg-[#CCFBF1]', border: 'border-[#0F766E]/40', text: 'text-[#0F766E]' },
};

export const SeatMapView: React.FC<SeatMapViewProps> = ({
  eventId,
  onBack,
  onContinue,
  onNotify,
}) => {
  const { data, isLoading, toggleSeat } = useSeatSelection(eventId, onNotify);
  const [zoom, setZoom] = useState(1);
  const [secondsLeft, setSecondsLeft] = useState(522);

  useEffect(() => {
    if (!data?.breakdown.holdExpiresAt) {
      setSecondsLeft(540);
      return;
    }
    const update = () => {
      const diff = Math.max(
        0,
        Math.floor((new Date(data.breakdown.holdExpiresAt!).getTime() - Date.now()) / 1000)
      );
      setSecondsLeft(diff);
    };
    update();
    const id = window.setInterval(update, 1000);
    return () => window.clearInterval(id);
  }, [data?.breakdown.holdExpiresAt]);

  const selectedSeats = useMemo(
    () => (data?.seats ?? []).filter((s) => s.status === 'SELECTED_BY_ME'),
    [data?.seats]
  );

  const groupedRows = useMemo(() => {
    const map = new Map<string, Seat[]>();
    for (const seat of data?.seats ?? []) {
      const list = map.get(seat.row) ?? [];
      list.push(seat);
      map.set(seat.row, list);
    }
    return Array.from(map.entries());
  }, [data?.seats]);

  if (isLoading || !data) {
    return (
      <div className="max-w-[1280px] mx-auto px-5 sm:px-10 py-10">
        <div className="h-[520px] rounded-[14px] bg-white border border-[#E7E5E0] animate-pulse" />
      </div>
    );
  }

  const { event, seats, breakdown } = data;
  const isClub = event.layoutType === 'CLUB';

  const renderSeatNode = (seat: Seat) => {
    const isSold = seat.status === 'SOLD';
    const isHeld = seat.status === 'HELD';
    const isSelected = seat.status === 'SELECTED_BY_ME';
    const tierStyle = TIER_PILL_COLOR[seat.tier];

    let cls = `${tierStyle.base} ${tierStyle.border} ${tierStyle.text} hover:brightness-95`;
    if (isSelected) {
      cls = 'bg-[#2D2A6E] border-[#2D2A6E] text-white shadow-card';
    } else if (isSold) {
      cls = 'bg-[#E7E5E0]/70 border-[#E7E5E0] text-[#5C6570]/60 cursor-not-allowed';
    } else if (isHeld) {
      cls = 'bg-[#FEF3C7]/40 border-[#D97706]/30 text-[#D97706] cursor-not-allowed';
    }

    return (
      <button
        key={seat.id}
        type="button"
        disabled={isSold || isHeld}
        onClick={() => toggleSeat.mutate(seat.id)}
        aria-label={`${seat.label} ${seat.tier} ${seat.status}`}
        title={
          seat.conflictDemo
            ? `${seat.label} (Live inventory)`
            : `${seat.label} · ${seat.tier}`
        }
        className={`h-10 min-w-[44px] px-2.5 rounded-[8px] border text-[12px] font-medium num flex items-center justify-center transition-colors duration-150 ${
          isSold || isHeld ? '' : 'cursor-pointer active:scale-95'
        } ${cls}`}
      >
        {isSold ? '×' : seat.label}
      </button>
    );
  };

  return (
    <div className="max-w-[1280px] mx-auto px-5 sm:px-10 pt-6 pb-32">
      {/* Top Bar */}
      <div className="flex flex-wrap items-center justify-between gap-4 pb-6 border-b border-[#E7E5E0]">
        <div className="flex items-center gap-3 min-w-0">
          <button
            type="button"
            onClick={onBack}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[8px] bg-white border border-[#E7E5E0] text-[13px] font-medium text-[#0F1419] hover:bg-[#F4F3F0] cursor-pointer active:scale-95 transition-all"
          >
            <ArrowLeft className="w-4 h-4" strokeWidth={1.75} />
            <span>Event</span>
          </button>
          <div className="min-w-0">
            <h1 className="text-[20px] sm:text-[24px] font-semibold text-[#0F1419] truncate">
              {event.title}
            </h1>
            <p className="text-[13px] text-[#5C6570] truncate">
              {event.dateLabel} · {event.timeLabel} · {event.venueName}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2 px-3.5 py-2 rounded-[8px] bg-white border border-[#E7E5E0]">
          <Clock className="w-4 h-4 text-[#B45309]" strokeWidth={1.75} />
          <span className="text-[12px] text-[#5C6570]">Hold window</span>
          <span className="num text-[14px] font-semibold text-[#0F1419]">
            {selectedSeats.length > 0 ? formatTimerClock(secondsLeft) : '09:00'}
          </span>
        </div>
      </div>

      {/* Tier Pricing & Status Legend */}
      <div className="flex flex-wrap items-center justify-between gap-4 py-4">
        <div className="flex flex-wrap items-center gap-4">
          {event.sections.map((sec) => (
            <div
              key={sec.id}
              className="flex items-center gap-2 px-3 py-1.5 rounded-[8px] bg-white border border-[#E7E5E0]"
            >
              <span className={`w-2.5 h-2.5 rounded-full ${TIER_PILL_COLOR[sec.tier].base} border ${TIER_PILL_COLOR[sec.tier].border}`} />
              <span className="text-[13px] font-medium text-[#0F1419]">{sec.name}</span>
              <span className="num text-[13px] font-semibold text-[#2D2A6E]">
                {formatINR(sec.price)}
              </span>
              <ZoneComplianceMarker isAlcoholic={sec.isAlcoholic} minAge={sec.minAge} />
            </div>
          ))}
        </div>

        <div className="flex items-center gap-4 text-[12px] text-[#5C6570]">
          <span className="inline-flex items-center gap-1.5">
            <span className="w-3 h-3 rounded-[4px] bg-[#CCFBF1] border border-[#0F766E]/40" />
            Available
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="w-3 h-3 rounded-[4px] bg-[#2D2A6E]" />
            Selected
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="w-3 h-3 rounded-[4px] bg-[#FEF3C7]/60 border border-[#D97706]/40" />
            Held
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="w-3 h-3 rounded-[4px] bg-[#E7E5E0]" />
            Sold
          </span>
        </div>
      </div>

      {/* Interactive Floor / Hall Map Viewport */}
      <div className="relative bg-white rounded-[14px] border border-[#E7E5E0] shadow-card p-6 sm:p-8 overflow-x-auto">
        <div className="flex items-center justify-between mb-6">
          <div className="text-[13px] font-medium text-[#5C6570]">
            {isClub ? 'Club & DJ Floor Plan — Bandra Warehouse' : 'Acoustic Hall Proscenium Layout'}
          </div>
          <div className="flex items-center gap-1.5">
            <button
              type="button"
              onClick={() => setZoom((z) => Math.max(0.85, Number((z - 0.1).toFixed(2))))}
              className="w-8 h-8 rounded-[8px] border border-[#E7E5E0] flex items-center justify-center text-[#0F1419] hover:bg-[#FAFAF7] cursor-pointer active:scale-95"
              aria-label="Zoom out seat map"
            >
              <Minus className="w-4 h-4" strokeWidth={1.75} />
            </button>
            <span className="num text-[12px] text-[#5C6570] w-12 text-center">
              {Math.round(zoom * 100)}%
            </span>
            <button
              type="button"
              onClick={() => setZoom((z) => Math.min(1.2, Number((z + 0.1).toFixed(2))))}
              className="w-8 h-8 rounded-[8px] border border-[#E7E5E0] flex items-center justify-center text-[#0F1419] hover:bg-[#FAFAF7] cursor-pointer active:scale-95"
              aria-label="Zoom in seat map"
            >
              <Plus className="w-4 h-4" strokeWidth={1.75} />
            </button>
            <button
              type="button"
              onClick={() => setZoom(1)}
              className="w-8 h-8 rounded-[8px] border border-[#E7E5E0] flex items-center justify-center text-[#0F1419] hover:bg-[#FAFAF7] cursor-pointer active:scale-95 ml-1"
              aria-label="Reset seat map zoom"
            >
              <RotateCcw className="w-3.5 h-3.5" strokeWidth={1.75} />
            </button>
          </div>
        </div>

        <div
          style={{ transform: `scale(${zoom})`, transformOrigin: 'top center' }}
          className="transition-transform duration-200 min-w-[680px]"
        >
          {isClub ? (
            /* Club & DJ Floor Layout matching mockup */
            <div className="space-y-8 max-w-4xl mx-auto">
              <div className="mx-auto max-w-md py-3 rounded-[10px] bg-[#FAFAF7] border border-[#E7E5E0] text-center">
                <div className="num text-[12px] font-semibold tracking-widest uppercase text-[#0F1419]">
                  DJ Booth &amp; Sound Rig
                </div>
                <div className="text-[11px] text-[#5C6570]">Quadraphonic Meyer Array</div>
              </div>

              <div className="grid grid-cols-12 gap-6 items-start">
                {/* Left VIP Booths */}
                <div className="col-span-3 p-4 rounded-[12px] bg-[#FEF3C7]/35 border border-[#B45309]/25 space-y-3">
                  <div className="text-[12px] font-semibold text-[#B45309] uppercase tracking-wider text-center">
                    West VIP Booths
                  </div>
                  <div className="grid grid-cols-1 gap-2">
                    {seats
                      .filter((s) => s.tier === 'VIP' && s.x <= 2)
                      .map((s) => renderSeatNode(s))}
                  </div>
                </div>

                {/* Center Couple Lounge + Central Dance Floor */}
                <div className="col-span-6 space-y-5">
                  <div className="p-4 rounded-[12px] bg-[#FCE7F0]/40 border border-[#E11D63]/25 space-y-3">
                    <div className="text-[12px] font-semibold text-[#E11D63] uppercase tracking-wider text-center">
                      Couple Lounge Zone
                    </div>
                    <div className="grid grid-cols-2 gap-2.5">
                      {seats.filter((s) => s.tier === 'COUPLE').map((s) => renderSeatNode(s))}
                    </div>
                  </div>

                  <div className="p-5 rounded-[12px] bg-[#CCFBF1]/35 border border-[#0F766E]/25 space-y-3">
                    <div className="text-[12px] font-semibold text-[#0F766E] uppercase tracking-wider text-center">
                      Central Dance Floor (Non-Alcoholic Tier)
                    </div>
                    <div className="grid grid-cols-2 gap-2.5">
                      {seats.filter((s) => s.tier === 'GENERAL').map((s) => renderSeatNode(s))}
                    </div>
                  </div>
                </div>

                {/* Right VIP Booths + Bar */}
                <div className="col-span-3 space-y-4">
                  <div className="p-4 rounded-[12px] bg-[#FEF3C7]/35 border border-[#B45309]/25 space-y-3">
                    <div className="text-[12px] font-semibold text-[#B45309] uppercase tracking-wider text-center">
                      East VIP Booths
                    </div>
                    <div className="grid grid-cols-1 gap-2">
                      {seats
                        .filter((s) => s.tier === 'VIP' && s.x > 2)
                        .map((s) => renderSeatNode(s))}
                    </div>
                  </div>

                  <div className="p-4 rounded-[12px] bg-[#FAFAF7] border border-dashed border-[#E7E5E0] text-center">
                    <div className="text-[12px] font-semibold text-[#0F1419]">Conservatory Bar</div>
                    <div className="text-[11px] text-[#5C6570] mt-0.5">F&amp;B Wallet Accepted</div>
                  </div>
                </div>
              </div>
            </div>
          ) : (
            /* Proscenium Acoustic Hall Layout */
            <div className="space-y-6 max-w-3xl mx-auto">
              <div className="mx-auto max-w-lg text-center">
                <svg
                  viewBox="0 0 420 28"
                  className="w-full h-7 text-[#2D2A6E]"
                  fill="none"
                  aria-hidden="true"
                >
                  <path
                    d="M12 24 C 120 4, 300 4, 408 24"
                    stroke="currentColor"
                    strokeWidth="2.5"
                    strokeLinecap="round"
                  />
                </svg>
                <div className="py-1 num text-[12px] font-semibold tracking-widest uppercase text-[#5C6570]">
                  Performance Stage
                </div>
              </div>

              <div className="space-y-3">
                {groupedRows.map(([rowName, rowSeats]) => {
                  const rowTier = rowSeats[0]?.tier ?? 'GENERAL';
                  return (
                    <div key={rowName} className="flex items-center justify-center gap-3">
                      <span className="num text-[12px] font-semibold text-[#5C6570] w-8 text-right">
                        {rowName}
                      </span>
                      <div className="grid grid-cols-8 gap-2">
                        {rowSeats.map((seat) => renderSeatNode(seat))}
                      </div>
                      <span className="text-[11px] font-medium text-[#5C6570] w-16">
                        {rowTier}
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Fixed Sticky Bottom Checkout Bar */}
      <div className="fixed inset-x-0 bottom-0 z-40 bg-white/95 backdrop-blur-sm border-t border-[#E7E5E0] shadow-modal">
        <div className="max-w-[1280px] mx-auto px-5 sm:px-10 py-3.5 flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-4">
          <div className="flex items-center gap-2 flex-wrap min-w-0">
            <span className="text-[13px] font-medium text-[#5C6570] mr-1">Selected:</span>
            {selectedSeats.length === 0 ? (
              <span className="text-[13px] text-[#5C6570]">
                Tap any available seat or booth above to lock your spot
              </span>
            ) : (
              selectedSeats.map((seat) => (
                <button
                  key={seat.id}
                  type="button"
                  onClick={() => toggleSeat.mutate(seat.id)}
                  className="inline-flex items-center gap-1.5 h-8 px-2.5 rounded-[8px] bg-[#FAFAF7] border border-[#E7E5E0] text-[12px] num font-medium text-[#0F1419] hover:border-[#DC2626] cursor-pointer active:scale-95 transition-all"
                >
                  <span>{seat.label}</span>
                  <X className="w-3.5 h-3.5 text-[#5C6570]" strokeWidth={1.75} />
                </button>
              ))
            )}
          </div>

          <div className="flex items-center justify-between sm:justify-end gap-5 shrink-0">
            <div className="text-left sm:text-right">
              <div className="text-[11px] text-[#5C6570] uppercase tracking-wider">
                Server Total ({breakdown.seatCount} {breakdown.seatCount === 1 ? 'seat' : 'seats'})
              </div>
              <div className="num text-[20px] font-semibold text-[#0F1419]">
                {formatINR(breakdown.totalPayable)}
              </div>
            </div>

            {/* ONE Primary CTA */}
            <Button
              variant="primary"
              size="lg"
              disabled={selectedSeats.length === 0}
              onClick={() => onContinue(event.id)}
            >
              <span>Continue to attendee details</span>
              <ArrowRight className="w-4 h-4" strokeWidth={1.75} />
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};
