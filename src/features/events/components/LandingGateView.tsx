import React from 'react';
import { motion } from 'motion/react';
import {
  ArrowRight,
  Building2,
  Calendar,
  MapPin,
  QrCode,
  Scissors,
  ShieldCheck,
  Sparkles,
  Ticket as TicketIcon,
  Wallet,
} from 'lucide-react';
import { SPRING_SPATIAL } from '../../../lib/motion/variants';
import { Button } from '../../../components/ui/Button';

interface LandingGateViewProps {
  onSignIn: () => void;
}

const CURATED_VENUES = [
  {
    name: 'Royal Opera House',
    city: 'Girgaon, Mumbai',
    layout: 'Proscenium Tiered Hall',
    capacity: '574 Seats',
    note: 'Restored 1916 Baroque acoustic hall built for unamplified classical strings, recitals, and intimate spoken word.',
  },
  {
    name: 'Antisocial Courtyard',
    city: 'Lower Parel, Mumbai',
    layout: '360° Club & Booth Floor',
    capacity: '320 Guests',
    note: 'Wraparound booth clusters and central DJ rig tuned for late-night electronic sets and table service.',
  },
  {
    name: 'NCPA Experimental Studio',
    city: 'Nariman Point, Mumbai',
    layout: 'Flexible Blackbox',
    capacity: '260 Seats',
    note: 'Steep acoustic raked seating with zero obstructed sightlines for contemporary theater and solo comedy tapings.',
  },
];

export const LandingGateView: React.FC<LandingGateViewProps> = ({ onSignIn }) => {
  return (
    <div className="max-w-[1280px] mx-auto px-5 sm:px-10 pt-10 sm:pt-16 pb-20">
      {/* Editorial Hero Split */}
      <section className="grid grid-cols-1 lg:grid-cols-12 gap-10 lg:gap-12 items-center">
        <motion.div
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          transition={SPRING_SPATIAL}
          className="lg:col-span-7 space-y-6"
        >
          <p className="text-[12px] font-medium tracking-[0.08em] uppercase text-[#5C6570]">
            City Culture &amp; Live Performances · Season 2026
          </p>
          <h1 className="text-[40px] sm:text-[56px] font-bold leading-[1.05] tracking-[-0.03em] text-[#0F1419] text-balance max-w-2xl">
            Live music, comedy, and intimate nights — crafted with paper precision.
          </h1>
          <p className="text-[17px] leading-[1.65] text-[#5C6570] max-w-xl">
            Reserve exact seats at historic listening rooms and architectural venues. Every pass
            carries a cryptographically signed QR, clean guest manifest, and venue bar credit.
          </p>

          <div className="pt-2 flex flex-wrap items-center gap-4">
            <Button variant="primary" size="lg" onClick={onSignIn}>
              <span>Sign In to Access Shows</span>
              <ArrowRight className="w-4 h-4" strokeWidth={1.75} />
            </Button>
          </div>
        </motion.div>

        {/* Right Editorial Perforated Mandala Ticket Preview (Static, No Wallet Amount, No Stag Label) */}
        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={SPRING_SPATIAL}
          className="lg:col-span-5 flex justify-center lg:justify-end"
        >
          <div className="w-full max-w-[460px] bg-white rounded-[16px] border border-[#E7E5E0] border-l-4 border-l-[#B45309] shadow-editorial overflow-hidden flex flex-col sm:flex-row relative">
            {/* Subtle Mandala Watermark */}
            <div
              aria-hidden="true"
              className="absolute inset-0 pointer-events-none opacity-[0.09] mix-blend-multiply"
              style={{
                backgroundImage:
                  "url('/assets/aistudio/Image_result_for_Fractal_Mandala_Rainbow_0611b49b.jpeg')",
                backgroundSize: 'cover',
                backgroundPosition: 'center',
              }}
            />
            <div
              aria-hidden="true"
              className="absolute inset-0 pointer-events-none bg-gradient-to-br from-[#FEF3C7]/40 via-white/70 to-white/90"
            />

            <div className="relative z-10 flex-1 p-6 space-y-4">
              <div className="flex items-center justify-between gap-2 pb-3 border-b border-dashed border-[#E7E5E0]">
                <span className="num text-[11px] font-semibold tracking-wider uppercase text-[#B45309] bg-[#FEF3C7] px-2.5 py-1 rounded-[6px]">
                  VIP Orchestra
                </span>
                <span className="num text-[11px] font-semibold text-[#5C6570]">
                  VNU-89421-VIP
                </span>
              </div>

              <div className="space-y-1.5">
                <div className="text-[12px] text-[#5C6570] flex items-center gap-1.5">
                  <MapPin className="w-3.5 h-3.5 text-[#B45309]" strokeWidth={1.75} />
                  <span>Royal Opera House · Gate A</span>
                </div>
                <h2 className="text-[20px] font-semibold leading-snug text-[#0F1419]">
                  Anoushka Shankar &amp; Guildhall Strings
                </h2>
              </div>

              <div className="grid grid-cols-2 gap-3 pt-3 border-t border-dashed border-[#E7E5E0] text-[12px]">
                <div>
                  <div className="text-[#5C6570] flex items-center gap-1">
                    <Calendar className="w-3 h-3" strokeWidth={1.75} />
                    <span>Schedule</span>
                  </div>
                  <div className="num font-medium text-[#0F1419] mt-0.5">28 Nov · 19:30 IST</div>
                </div>
                <div>
                  <div className="text-[#5C6570]">Seats (2)</div>
                  <div className="num font-medium text-[#0F1419] mt-0.5">A-1, A-2</div>
                </div>
              </div>

              <div className="flex items-center justify-between pt-2 text-[11px] text-[#5C6570]">
                <span>Guests: Arjun Mehta, Pooja Mehta</span>
                <span className="num font-semibold text-[#2D2A6E]">CODE BAR-8942</span>
              </div>
            </div>

            {/* Right QR Tear Stub */}
            <div className="relative z-10 sm:w-[134px] shrink-0 bg-[#FAFAF7]/90 border-t sm:border-t-0 sm:border-l-2 border-dashed border-[#D6D3CC] p-4 flex flex-row sm:flex-col items-center justify-between gap-2 text-center">
              <div className="flex items-center gap-1 num text-[10px] font-semibold uppercase tracking-wider text-[#5C6570]">
                <Scissors className="w-3 h-3" strokeWidth={1.75} />
                <span>Admit 2</span>
              </div>
              <div className="w-16 h-16 rounded-[10px] bg-white border border-[#E7E5E0] flex items-center justify-center text-[#0F1419]">
                <QrCode className="w-9 h-9" strokeWidth={1.5} />
              </div>
              <div className="num text-[10px] text-[#5C6570]">Tears at Door</div>
            </div>
          </div>
        </motion.div>
      </section>

      {/* Editorial Proof Strip */}
      <section className="grid grid-cols-1 sm:grid-cols-3 gap-6 py-8 my-14 border-y border-[#E7E5E0]">
        <div>
          <div className="num text-[26px] font-semibold text-[#0F1419]">100%</div>
          <div className="text-[13px] text-[#5C6570] mt-1">
            Zero-Trust Server Pricing With Atomic 9-Minute Seat Holds
          </div>
        </div>
        <div>
          <div className="num text-[26px] font-semibold text-[#0F1419]">HMAC-SHA256</div>
          <div className="text-[13px] text-[#5C6570] mt-1">
            Cryptographic Entry Passes With Single-Use Perforated Gate Tears
          </div>
        </div>
        <div>
          <div className="num text-[26px] font-semibold text-[#0F1419]">0%</div>
          <div className="text-[13px] text-[#5C6570] mt-1">
            Hidden Gate Surcharges or Surprise Checkout Markups
          </div>
        </div>
      </section>

      {/* Curated Architectural Spaces */}
      <section className="space-y-6">
        <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-4">
          <div>
            <p className="text-[11px] font-semibold uppercase tracking-widest text-[#5C6570] num">
              Architectural Spaces
            </p>
            <h2 className="text-[28px] font-semibold text-[#0F1419] mt-1">
              Partner rooms tuned for live listening
            </h2>
          </div>
          <div className="inline-flex items-center gap-1.5 text-[12px] font-medium text-[#2D2A6E]">
            <Sparkles className="w-4 h-4" strokeWidth={1.75} />
            <span>Sign in to view upcoming season lineups</span>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {CURATED_VENUES.map((room) => (
            <div
              key={room.name}
              className="bg-white rounded-[14px] border border-[#E7E5E0] p-6 space-y-4 flex flex-col justify-between shadow-card"
            >
              <div className="space-y-2.5">
                <div className="flex items-center justify-between gap-2">
                  <Building2 className="w-5 h-5 text-[#2D2A6E]" strokeWidth={1.75} />
                  <span className="num text-[12px] font-medium text-[#5C6570]">
                    {room.capacity}
                  </span>
                </div>
                <h3 className="text-[18px] font-semibold text-[#0F1419]">{room.name}</h3>
                <div className="text-[12.5px] font-medium text-[#2D2A6E]">{room.layout}</div>
                <p className="text-[13px] text-[#5C6570] leading-relaxed">{room.note}</p>
              </div>
              <div className="pt-3 border-t border-[#E7E5E0] flex items-center gap-1.5 text-[12px] text-[#5C6570]">
                <MapPin className="w-3.5 h-3.5" strokeWidth={1.75} />
                <span>{room.city}</span>
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* How Venu Works — 3 Architectural Steps */}
      <section className="mt-16 pt-12 border-t border-[#E7E5E0] space-y-6">
        <div className="max-w-xl">
          <p className="text-[11px] font-semibold uppercase tracking-widest text-[#5C6570] num">
            From Seat Map to Door
          </p>
          <h2 className="text-[28px] font-semibold text-[#0F1419] mt-1">
            Built for calm arrivals and zero bar queues
          </h2>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          <div className="bg-white rounded-[14px] border border-[#E7E5E0] p-6 space-y-3">
            <div className="flex items-center justify-between">
              <span className="num text-[12px] font-semibold text-[#2D2A6E]">01</span>
              <TicketIcon className="w-5 h-5 text-[#2D2A6E]" strokeWidth={1.75} />
            </div>
            <h3 className="text-[16px] font-semibold text-[#0F1419]">Exact Seat Selection</h3>
            <p className="text-[13px] text-[#5C6570] leading-relaxed">
              Lock your preferred row, couple booth, or club table atomically for 9 minutes while
              entering verified guest details.
            </p>
          </div>

          <div className="bg-white rounded-[14px] border border-[#E7E5E0] p-6 space-y-3">
            <div className="flex items-center justify-between">
              <span className="num text-[12px] font-semibold text-[#0F766E]">02</span>
              <Wallet className="w-5 h-5 text-[#0F766E]" strokeWidth={1.75} />
            </div>
            <h3 className="text-[16px] font-semibold text-[#0F1419]">Private Bar Wallet</h3>
            <p className="text-[13px] text-[#5C6570] leading-relaxed">
              Open your digital pass to view your venue F&amp;B credit balance, validity window, and
              personal bar code.
            </p>
          </div>

          <div className="bg-white rounded-[14px] border border-[#E7E5E0] p-6 space-y-3">
            <div className="flex items-center justify-between">
              <span className="num text-[12px] font-semibold text-[#B45309]">03</span>
              <ShieldCheck className="w-5 h-5 text-[#B45309]" strokeWidth={1.75} />
            </div>
            <h3 className="text-[16px] font-semibold text-[#0F1419]">Perforated Gate Pass</h3>
            <p className="text-[13px] text-[#5C6570] leading-relaxed">
              Present your signed QR pass at entry. Once verified, only the entry QR stub tears away
              cleanly.
            </p>
          </div>
        </div>
      </section>

      {/* Bottom Sign-In Editorial Banner */}
      <section className="mt-16 bg-white rounded-[16px] border border-[#E7E5E0] shadow-card p-8 sm:p-10 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-6">
        <div className="space-y-1.5 max-w-xl">
          <h2 className="text-[22px] sm:text-[26px] font-semibold text-[#0F1419]">
            Ready for tonight’s lineup?
          </h2>
          <p className="text-[14px] text-[#5C6570]">
            Sign in to explore live concerts, comedy tapings, and acoustic sessions.
          </p>
        </div>
        <Button variant="primary" size="lg" onClick={onSignIn}>
          <span>Sign In</span>
          <ArrowRight className="w-4 h-4" strokeWidth={1.75} />
        </Button>
      </section>
    </div>
  );
};
