import React, { useEffect, useState } from 'react';
import { CheckCircle2, QrCode, ScanLine, Search, ShieldAlert, UserCheck } from 'lucide-react';
import type { ScanLookupResult } from '../../../lib/api/types';
import { Button } from '../../../components/ui/Button';
import { PaperTicketCard } from '../../booking/components/PaperTicketCard';
import { useScanner } from '../hooks/useScanner';

interface GateScannerViewProps {
  onNotify: (title: string, tone: 'success' | 'error' | 'info') => void;
}

export const GateScannerView: React.FC<GateScannerViewProps> = ({ onNotify }) => {
  const { recentScans, allTickets, lookupTicket, admitTicket } = useScanner();
  const [query, setQuery] = useState('VNU-89421-MNT');
  const [scanResult, setScanResult] = useState<ScanLookupResult | null>(null);

  const runLookup = async (codeToLookup: string) => {
    try {
      const res = await lookupTicket.mutateAsync(codeToLookup);
      setScanResult(res);
    } catch (err) {
      onNotify(err instanceof Error ? err.message : 'Scan lookup failed', 'error');
    }
  };

  useEffect(() => {
    if (!scanResult) {
      void runLookup('VNU-89421-MNT');
    }
  }, []);

  const handleAdmitAll = async () => {
    if (!scanResult?.ticket) return;
    try {
      const updated = await admitTicket.mutateAsync({ ticketId: scanResult.ticket.id });
      setScanResult({
        valid: false,
        reason: 'ALREADY_CHECKED_IN',
        ticket: updated,
      });
      onNotify(`Admitted ${updated.code} · Gate stub torn`, 'success');
    } catch (err) {
      onNotify(err instanceof Error ? err.message : 'Check-in failed', 'error');
    }
  };

  const handleAdmitIndividual = async (seatId: string) => {
    if (!scanResult?.ticket) return;
    try {
      const updated = await admitTicket.mutateAsync({
        ticketId: scanResult.ticket.id,
        seatId,
      });
      setScanResult({
        valid: !updated.checkedIn,
        reason: updated.checkedIn ? 'ALREADY_CHECKED_IN' : 'VALID',
        ticket: updated,
      });
      onNotify(
        updated.checkedIn ? 'All guests admitted · Gate stub torn' : 'Guest marked admitted.',
        'success'
      );
    } catch (err) {
      onNotify(err instanceof Error ? err.message : 'Failed to admit guest', 'error');
    }
  };

  return (
    <div className="max-w-[1280px] mx-auto px-5 sm:px-10 pt-8 pb-16">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-4 pb-6 border-b border-[#E7E5E0]">
        <div>
          <div className="text-[12px] font-medium text-[#5C6570] uppercase tracking-wider">
            Gate A · Main Entrance · Terminal 01
          </div>
          <h1 className="text-[28px] font-semibold text-[#0F1419] mt-1">Gate Check-In Scanner</h1>
        </div>

        <form
          onSubmit={(e) => {
            e.preventDefault();
            void runLookup(query);
          }}
          className="flex items-center gap-2 w-full sm:w-auto"
        >
          <div className="relative flex-1 sm:w-64">
            <Search
              className="w-4 h-4 text-[#5C6570] absolute left-3 top-1/2 -translate-y-1/2"
              strokeWidth={1.75}
            />
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Ticket code or HMAC QR..."
              aria-label="Ticket code lookup"
              className="w-full h-11 pl-9 pr-3 rounded-[8px] bg-white border border-[#E7E5E0] text-[13px] num focus:outline-none focus:border-[#2D2A6E]"
            />
          </div>
          <Button type="submit" variant="secondary" isLoading={lookupTicket.isPending}>
            Scan
          </Button>
        </form>
      </div>

      {/* Main 12-Col Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 mt-8 items-start">
        {/* Left: Optical Scanner Frame & Quick-Scan Selector */}
        <div className="lg:col-span-5 space-y-5">
          <div className="bg-white rounded-[14px] border border-[#E7E5E0] shadow-card p-6 space-y-5">
            <div className="aspect-[4/3] rounded-[12px] bg-[#FAFAF7] border-2 border-dashed border-[#2D2A6E]/30 flex flex-col items-center justify-center p-6 text-center relative">
              <div className="w-16 h-16 rounded-[14px] bg-white border border-[#E7E5E0] flex items-center justify-center text-[#2D2A6E] shadow-card mb-3">
                <ScanLine className="w-8 h-8" strokeWidth={1.75} />
              </div>
              <div className="text-[15px] font-semibold text-[#0F1419]">
                Optical QR Lens Ready
              </div>
              <p className="text-[13px] text-[#5C6570] mt-1 max-w-xs">
                Point patron mobile pass at scanner or select a pass code below to verify.
              </p>
            </div>

            <div className="space-y-2">
              <div className="text-[12px] font-medium text-[#5C6570] uppercase tracking-wider">
                Live Venue Passes
              </div>
              <div className="flex flex-wrap gap-2">
                {allTickets.slice(0, 4).map((tkt) => (
                  <button
                    key={tkt.id}
                    type="button"
                    onClick={() => {
                      setQuery(tkt.code);
                      void runLookup(tkt.code);
                    }}
                    className={`px-3 py-1.5 rounded-[8px] border text-[12px] num font-medium cursor-pointer active:scale-95 transition-all ${
                      scanResult?.ticket?.id === tkt.id
                        ? 'bg-[#2D2A6E] text-white border-[#2D2A6E]'
                        : 'bg-[#FAFAF7] text-[#0F1419] border-[#E7E5E0] hover:bg-white'
                    }`}
                  >
                    {tkt.code}
                  </button>
                ))}
                <button
                  type="button"
                  onClick={() => {
                    setQuery('eyJ0aWNrZXRfaWQiOiJ0a3RfZmFrZSJ9.tampered_hmac_sig');
                    void runLookup('eyJ0aWNrZXRfaWQiOiJ0a3RfZmFrZSJ9.tampered_hmac_sig');
                  }}
                  className="px-3 py-1.5 rounded-[8px] border border-[#DC2626]/30 bg-[#FEE2E2]/40 text-[#DC2626] text-[12px] num font-medium cursor-pointer active:scale-95 transition-all"
                >
                  Tampered QR
                </button>
              </div>
            </div>
          </div>

          {/* Recent Gate Log */}
          <div className="bg-white rounded-[14px] border border-[#E7E5E0] p-6 space-y-4">
            <h2 className="text-[16px] font-semibold text-[#0F1419]">Recent Gate Check-Ins</h2>
            <div className="divide-y divide-[#E7E5E0]">
              {recentScans.slice(0, 5).map((scan) => (
                <div key={scan.id} className="py-3 flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <div className="text-[14px] font-medium text-[#0F1419] truncate">
                      {scan.patronName}
                    </div>
                    <div className="num text-[12px] text-[#5C6570]">
                      {scan.ticketCode} · {scan.seatLabel}
                    </div>
                  </div>
                  <div className="text-right shrink-0">
                    <div className="num text-[12px] font-medium text-[#16A34A]">Admitted</div>
                    <div className="num text-[11px] text-[#5C6570]">{scan.scannedAt}</div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Right: Paper Ticket Visual (Tears Stub on Admit) & Verification Controls */}
        <div className="lg:col-span-7 space-y-6">
          {!scanResult ? (
            <div className="bg-white rounded-[14px] border border-[#E7E5E0] p-12 text-center text-[#5C6570]">
              <QrCode className="w-8 h-8 mx-auto mb-3 text-[#5C6570]" strokeWidth={1.75} />
              <p className="text-[15px]">Awaiting QR scan or ticket code lookup.</p>
            </div>
          ) : !scanResult.ticket ||
            scanResult.reason === 'EXPIRED' ||
            scanResult.reason === 'ROTATING_QR_EXPIRED' ? (
            <div className="bg-white rounded-[14px] border border-[#DC2626]/30 p-8 space-y-4">
              <div className="inline-flex items-center gap-2 px-3 py-1.5 rounded-[6px] bg-[#FEE2E2] text-[#DC2626] text-[13px] font-semibold num">
                <ShieldAlert className="w-4 h-4" strokeWidth={1.75} />
                <span>
                  {scanResult.reason === 'EXPIRED'
                    ? 'PASS EXPIRED · ENTRY DENIED'
                    : scanResult.reason === 'ROTATING_QR_EXPIRED'
                      ? 'ROTATING CODE EXPIRED · REFRESH PASS'
                      : 'INVALID HMAC SIGNATURE · ENTRY DENIED'}
                </span>
              </div>
              <h2 className="text-[22px] font-semibold text-[#0F1419]">
                {scanResult.reason === 'EXPIRED'
                  ? 'Ticket validity window has closed'
                  : scanResult.reason === 'ROTATING_QR_EXPIRED'
                    ? 'Rotating 30-second QR code is out of sync'
                    : 'Unrecognized or forged QR token'}
              </h2>
              <p className="text-[14px] text-[#5C6570]">
                {scanResult.reason === 'ROTATING_QR_EXPIRED'
                  ? 'Ask the patron to wait for their 30-second rotating pass to refresh and scan again.'
                  : 'Cryptographic HMAC-SHA256 verification failed. Direct patron to the Box Office desk.'}
              </p>
            </div>
          ) : (
            <div className="space-y-5">
              {/* Verification Banner */}
              <div
                className={`px-5 py-3.5 rounded-[12px] border flex flex-wrap items-center justify-between gap-3 ${
                  scanResult.reason === 'VALID'
                    ? 'bg-[#DCFCE7]/60 border-[#16A34A]/25'
                    : 'bg-[#FEF3C7]/70 border-[#B45309]/25'
                }`}
              >
                <div className="flex items-center gap-2.5">
                  {scanResult.reason === 'VALID' ? (
                    <CheckCircle2 className="w-5 h-5 text-[#16A34A]" strokeWidth={1.75} />
                  ) : (
                    <ShieldAlert className="w-5 h-5 text-[#B45309]" strokeWidth={1.75} />
                  )}
                  <span
                    className={`num text-[14px] font-semibold tracking-wide ${
                      scanResult.reason === 'VALID' ? 'text-[#16A34A]' : 'text-[#B45309]'
                    }`}
                  >
                    {scanResult.reason === 'VALID'
                      ? `VALID PASS · ${scanResult.ticket.code}`
                      : `SCANNED & TORN · ${scanResult.ticket.code}`}
                  </span>
                </div>
                <span className="num text-[12px] text-[#5C6570]">
                  {scanResult.ticket.seatLabels.length}{' '}
                  {scanResult.ticket.seatLabels.length === 1 ? 'Seat' : 'Seats'} Booked
                </span>
              </div>

              {/* Physical Ticket Card (Automatically rips end stub once checkedIn) */}
              <PaperTicketCard
                ticket={scanResult.ticket}
                actionLabel={!scanResult.ticket.checkedIn ? 'Admit & Tear' : undefined}
                onAction={!scanResult.ticket.checkedIn ? handleAdmitAll : undefined}
                actionLoading={admitTicket.isPending}
              />

              {/* Per-Guest Gate Checklist while unscanned */}
              {!scanResult.ticket.checkedIn && (
                <div className="bg-white rounded-[14px] border border-[#E7E5E0] p-6 space-y-4">
                  <div className="text-[13px] font-semibold text-[#0F1419]">
                    Verify Guests ({scanResult.ticket.attendees.length}{' '}
                    {scanResult.ticket.attendees.length === 1 ? 'Guest' : 'Guests'})
                  </div>
                  <div className="divide-y divide-[#E7E5E0] border border-[#E7E5E0] rounded-[12px] overflow-hidden">
                    {scanResult.ticket.attendees.map((att) => (
                      <div
                        key={att.seatId}
                        className="p-4 bg-[#FAFAF7]/50 flex items-center justify-between gap-4"
                      >
                        <div>
                          <div className="text-[15px] font-semibold text-[#0F1419]">
                            {att.fullName}
                          </div>
                          <div className="num text-[12px] text-[#5C6570] mt-0.5">
                            Seat {att.seatLabel} · Age {att.age}
                            {att.gender ? ` · ${att.gender}` : ''}
                          </div>
                        </div>

                        {att.admitted ? (
                          <span className="inline-flex items-center gap-1.5 text-[13px] font-medium text-[#16A34A]">
                            <CheckCircle2 className="w-4 h-4" strokeWidth={1.75} />
                            <span>Admitted</span>
                          </span>
                        ) : (
                          <Button
                            variant="secondary"
                            size="sm"
                            isLoading={admitTicket.isPending}
                            onClick={() => handleAdmitIndividual(att.seatId)}
                          >
                            Admit Seat
                          </Button>
                        )}
                      </div>
                    ))}
                  </div>

                  {/* Single Primary CTA */}
                  <Button
                    variant="primary"
                    size="lg"
                    className="w-full"
                    isLoading={admitTicket.isPending}
                    onClick={handleAdmitAll}
                  >
                    <UserCheck className="w-4 h-4" strokeWidth={1.75} />
                    <span>Admit All Guests &amp; Tear Stub</span>
                  </Button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
