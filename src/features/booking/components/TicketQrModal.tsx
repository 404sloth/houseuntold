import React, { useCallback, useEffect, useState } from 'react';
import { AnimatePresence, motion } from 'motion/react';
import { QRCodeSVG } from 'qrcode.react';
import {
  CheckCircle2,
  Clock,
  KeyRound,
  RefreshCw,
  ShieldCheck,
  Users,
  Wallet,
  X,
} from 'lucide-react';
import { api } from '../../../lib/api';
import type { Ticket } from '../../../lib/api/types';
import { formatINR, isTicketExpired } from '../../../lib/domain/rules';
import { SPRING_SPATIAL } from '../../../lib/motion/variants';
import { TierBadge } from '../../../components/ui/TierBadge';

interface TicketQrModalProps {
  ticket: Ticket | null;
  onClose: () => void;
}

function formatExpiryLabel(expiresAt: string): string {
  const ms = Date.parse(expiresAt);
  if (Number.isNaN(ms)) return expiresAt;
  return new Date(ms).toLocaleString('en-IN', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export const TicketQrModal: React.FC<TicketQrModalProps> = ({ ticket, onClose }) => {
  const [rotating, setRotating] = useState(false);
  const [qrToken, setQrToken] = useState<string>('');
  const [countdown, setCountdown] = useState<number>(30);
  const [loadingQr, setLoadingQr] = useState(false);

  const loadSignedQr = useCallback(async (ticketId: string, useRotating: boolean) => {
    setLoadingQr(true);
    try {
      const res = await api.getTicketQr(ticketId, { rotating: useRotating });
      setQrToken(res.qrToken);
      if (typeof res.expiresInSeconds === 'number') {
        setCountdown(res.expiresInSeconds);
      }
    } catch {
      // Fallback handled by initial ticket.qrToken if offline
    } finally {
      setLoadingQr(false);
    }
  }, []);

  useEffect(() => {
    if (!ticket || ticket.checkedIn) {
      setQrToken('');
      return;
    }
    setQrToken(ticket.qrToken);
    void loadSignedQr(ticket.id, rotating);
  }, [ticket, rotating, loadSignedQr]);

  useEffect(() => {
    if (!ticket || ticket.checkedIn || !rotating) return;
    const interval = window.setInterval(() => {
      const rem = 30 - (Math.floor(Date.now() / 1000) % 30);
      setCountdown(rem);
      if (rem === 30) {
        void loadSignedQr(ticket.id, true);
      }
    }, 1000);
    return () => window.clearInterval(interval);
  }, [ticket, rotating, loadSignedQr]);

  const activeQrValue = qrToken || ticket?.qrToken || '';
  const expired = ticket ? isTicketExpired(ticket) : false;

  return (
    <AnimatePresence>
      {ticket && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-[#0F1419]/40 backdrop-blur-[2px]"
          role="dialog"
          aria-modal="true"
          aria-labelledby="qr-modal-title"
          onClick={onClose}
        >
          <motion.div
            initial={{ opacity: 0, scale: 0.94 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.94 }}
            transition={SPRING_SPATIAL}
            onClick={(e) => e.stopPropagation()}
            className="w-full max-w-md bg-white rounded-[20px] border border-[#E7E5E0] shadow-modal overflow-hidden max-h-[90vh] flex flex-col"
          >
            <div className="p-6 border-b border-dashed border-[#E7E5E0] flex items-start justify-between gap-4 shrink-0">
              <div>
                <div className="flex flex-wrap items-center gap-2">
                  <span className="num text-[12px] font-semibold text-[#5C6570]">
                    {ticket.code}
                  </span>
                  <TierBadge tier={ticket.tier} label={ticket.sectionName} />
                </div>
                <h2
                  id="qr-modal-title"
                  className="text-[20px] font-semibold text-[#0F1419] mt-2"
                >
                  {ticket.eventTitle}
                </h2>
                <p className="text-[13px] text-[#5C6570] mt-0.5">
                  {ticket.dateLabel} · {ticket.venueName}
                </p>
              </div>

              <button
                type="button"
                onClick={onClose}
                className="p-1.5 rounded-[8px] text-[#5C6570] hover:text-[#0F1419] hover:bg-[#FAFAF7] cursor-pointer active:scale-95 transition-all"
                aria-label="Close pass details"
              >
                <X className="w-5 h-5" strokeWidth={1.75} />
              </button>
            </div>

            <div className="p-6 space-y-5 overflow-y-auto">
              {/* F&B Wallet Balance + Expiry (Revealed Only Here) */}
              <div
                className={`p-4 rounded-[12px] border space-y-2 ${
                  expired
                    ? 'bg-[#FEE2E2]/45 border-[#DC2626]/25'
                    : 'bg-[#CCFBF1]/35 border-[#0F766E]/25'
                }`}
              >
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2 text-[13px] font-semibold text-[#0F1419]">
                    <Wallet
                      className={`w-4 h-4 ${expired ? 'text-[#DC2626]' : 'text-[#0F766E]'}`}
                      strokeWidth={1.75}
                    />
                    <span>F&amp;B Bar Wallet</span>
                  </div>
                  <span
                    className={`num text-[11px] font-semibold px-2 py-0.5 rounded-[6px] ${
                      expired
                        ? 'bg-[#FEE2E2] text-[#DC2626]'
                        : 'bg-white text-[#0F766E] border border-[#0F766E]/25'
                    }`}
                  >
                    {expired ? 'EXPIRED' : 'ACTIVE'}
                  </span>
                </div>

                <div className="flex items-baseline justify-between gap-2">
                  <span
                    className={`num text-[24px] font-bold ${
                      expired ? 'text-[#5C6570] line-through' : 'text-[#0F766E]'
                    }`}
                  >
                    {formatINR(ticket.foodBalance)}
                  </span>
                  <span className="num inline-flex items-center gap-1 text-[12px] font-semibold text-[#2D2A6E] bg-white px-2.5 py-1 rounded-[6px] border border-[#E7E5E0]">
                    <KeyRound className="w-3.5 h-3.5" strokeWidth={1.75} />
                    <span>Bar Code: {ticket.barCode}</span>
                  </span>
                </div>

                <div className="flex items-center gap-1.5 text-[11.5px] text-[#5C6570] num">
                  <Clock className="w-3.5 h-3.5 shrink-0" strokeWidth={1.75} />
                  <span>
                    {expired
                      ? `Wallet expired on ${formatExpiryLabel(ticket.expiresAt)}`
                      : `Valid until ${formatExpiryLabel(ticket.expiresAt)}`}
                  </span>
                </div>
              </div>

              {/* QR Section or Admitted Confirmation */}
              {ticket.checkedIn ? (
                <div className="p-5 rounded-[14px] bg-[#FAFAF7] border border-[#E7E5E0] text-center space-y-2">
                  <CheckCircle2 className="w-6 h-6 text-[#0F766E] mx-auto" strokeWidth={1.75} />
                  <div className="text-[15px] font-semibold text-[#0F1419]">
                    Pass Scanned &amp; Gate Stub Detached
                  </div>
                  <p className="text-[13px] text-[#5C6570]">
                    Entry QR has been redeemed for {ticket.seatLabels.length}{' '}
                    {ticket.seatLabels.length === 1 ? 'guest' : 'guests'}. Share your Bar Code{' '}
                    <span className="num font-semibold text-[#2D2A6E]">{ticket.barCode}</span> at
                    any venue bar counter before expiry.
                  </p>
                </div>
              ) : (
                <div className="flex flex-col items-center p-5 rounded-[14px] bg-[#FAFAF7] border border-[#E7E5E0] space-y-3">
                  <QRCodeSVG
                    value={activeQrValue}
                    size={190}
                    bgColor="#FAFAF7"
                    fgColor="#0F1419"
                  />

                  <div className="flex items-center gap-1.5 text-[11px] font-medium text-[#0F766E]">
                    <ShieldCheck className="w-3.5 h-3.5" strokeWidth={1.75} />
                    <span className="num">HMAC-SHA256 Signed · {ticket.code}</span>
                  </div>

                  <div className="w-full pt-2 border-t border-[#E7E5E0] flex items-center justify-between gap-2">
                    <button
                      type="button"
                      onClick={() => setRotating((prev) => !prev)}
                      className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-[6px] text-[11px] font-medium border cursor-pointer active:scale-95 transition-all ${
                        rotating
                          ? 'bg-[#2D2A6E] text-white border-[#2D2A6E]'
                          : 'bg-white text-[#5C6570] border-[#E7E5E0] hover:text-[#0F1419]'
                      }`}
                    >
                      <RefreshCw
                        className={`w-3 h-3 ${loadingQr ? 'animate-spin' : ''}`}
                        strokeWidth={1.75}
                      />
                      <span>{rotating ? 'Rotating 30s QR On' : 'Enable Rotating 30s QR'}</span>
                    </button>

                    {rotating && (
                      <span className="num text-[11px] font-semibold text-[#B45309]">
                        Rotates in {String(countdown).padStart(2, '0')}s
                      </span>
                    )}
                  </div>
                </div>
              )}

              {/* Gate, Seats & Guest Manifest */}
              <div className="grid grid-cols-2 gap-4 text-[13px]">
                <div>
                  <div className="text-[#5C6570]">Gate</div>
                  <div className="num font-medium text-[#0F1419] mt-0.5">{ticket.gate}</div>
                </div>
                <div>
                  <div className="text-[#5C6570]">Seats ({ticket.seatLabels.length})</div>
                  <div className="num font-medium text-[#0F1419] mt-0.5">
                    {ticket.seatLabels.join(', ')}
                  </div>
                </div>
              </div>

              <div className="p-3.5 rounded-[10px] bg-[#FAFAF7] border border-[#E7E5E0] space-y-2">
                <div className="flex items-center gap-1.5 text-[12px] font-semibold text-[#5C6570]">
                  <Users className="w-3.5 h-3.5" strokeWidth={1.75} />
                  <span>Guest Manifest</span>
                </div>
                <div className="divide-y divide-[#E7E5E0]">
                  {ticket.attendees.map((att) => (
                    <div
                      key={att.seatId}
                      className="py-1.5 flex items-center justify-between text-[13px]"
                    >
                      <span className="font-medium text-[#0F1419]">{att.fullName}</span>
                      <span className="num text-[12px] text-[#5C6570]">
                        Seat {att.seatLabel} · Age {att.age}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </motion.div>
        </div>
      )}
    </AnimatePresence>
  );
};
