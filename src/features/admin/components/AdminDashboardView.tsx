import React, { useMemo, useState } from 'react';
import { AnimatePresence, motion } from 'motion/react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  Check,
  Copy,
  Eye,
  KeyRound,
  Plus,
  Search,
  ShieldCheck,
  UserPlus,
  Users,
  X,
} from 'lucide-react';
import { QRCodeSVG } from 'qrcode.react';
import type { AdminMfaEnrollmentResponse } from '../../../lib/api/types';
import {
  CreateEventFormSchema,
  CreateStaffUserInputSchema,
  type CreateEventForm,
  type CreateStaffUserInput,
} from '../../../lib/api/schemas';
import { formatINR } from '../../../lib/domain/rules';
import { SPRING_SPATIAL } from '../../../lib/motion/variants';
import { Button } from '../../../components/ui/Button';
import { useAdmin } from '../hooks/useAdmin';

interface AdminDashboardViewProps {
  onInspectSeatMap: (eventId: string) => void;
  onNotify: (title: string, tone: 'success' | 'error' | 'info') => void;
}

export const AdminDashboardView: React.FC<AdminDashboardViewProps> = ({
  onInspectSeatMap,
  onNotify,
}) => {
  const { stats, events, staffUsers, updateStatus, createEvent, createStaff, enrollMfa } =
    useAdmin();
  const [search, setSearch] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [staffOpen, setStaffOpen] = useState(false);
  const [issuedCredentials, setIssuedCredentials] = useState<{
    name: string;
    email: string;
    password: string;
    role: 'SCANNER' | 'BARTENDER';
  } | null>(null);
  const [mfaEnrollment, setMfaEnrollment] = useState<AdminMfaEnrollmentResponse | null>(null);
  const [copiedSecret, setCopiedSecret] = useState(false);

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<CreateEventForm>({
    resolver: zodResolver(CreateEventFormSchema),
    defaultValues: {
      title: 'NCPA Jazz & Brass Quartet',
      subtitle: 'An intimate evening of acoustic jazz and brass improvisation in South Mumbai.',
      category: 'Live Music',
      venueName: 'Tata Theatre, NCPA',
      dateLabel: 'Sat, 13 Dec 2026',
      timeLabel: '19:30 IST',
      layoutType: 'PROSCENIUM',
      vipPrice: 2000,
      generalPrice: 1100,
    },
  });

  const staffForm = useForm<CreateStaffUserInput>({
    resolver: zodResolver(CreateStaffUserInputSchema),
    defaultValues: {
      name: '',
      email: '',
      password: '',
      role: 'SCANNER',
    },
  });

  const filteredEvents = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return events;
    return events.filter(
      (e) =>
        e.title.toLowerCase().includes(q) ||
        e.venueName.toLowerCase().includes(q) ||
        e.category.toLowerCase().includes(q)
    );
  }, [events, search]);

  const handleCreateSubmit = async (values: CreateEventForm) => {
    try {
      const created = await createEvent.mutateAsync(values);
      onNotify(`Published "${created.title}" with 24 live seats.`, 'success');
      reset();
      setCreateOpen(false);
    } catch (err) {
      onNotify(err instanceof Error ? err.message : 'Could not create event', 'error');
    }
  };

  const handleCreateStaffSubmit = async (values: CreateStaffUserInput) => {
    try {
      const created = await createStaff.mutateAsync(values);
      setIssuedCredentials({
        name: created.name,
        email: created.email,
        password: values.password,
        role: created.role,
      });
      staffForm.reset();
      setStaffOpen(false);
      onNotify(`Provisioned ${created.role} credentials for ${created.name}.`, 'success');
    } catch (err) {
      onNotify(err instanceof Error ? err.message : 'Could not create staff ID', 'error');
    }
  };

  return (
    <div className="max-w-[1280px] mx-auto px-5 sm:px-10 pt-8 pb-16 space-y-8">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-4 pb-6 border-b border-[#E7E5E0]">
        <div>
          <div className="text-[12px] font-medium text-[#5C6570] uppercase tracking-wider">
            Royal Opera House &amp; City Venues · Shift Manager
          </div>
          <h1 className="text-[28px] font-semibold text-[#0F1419] mt-1">Operations Console</h1>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="secondary"
            size="md"
            isLoading={enrollMfa.isPending}
            onClick={async () => {
              try {
                const res = await enrollMfa.mutateAsync();
                setCopiedSecret(false);
                setMfaEnrollment(res);
              } catch (err) {
                onNotify(err instanceof Error ? err.message : 'MFA enrollment failed', 'error');
              }
            }}
          >
            <ShieldCheck className="w-4 h-4 text-[#2D2A6E]" strokeWidth={1.75} />
            <span>Authenticator MFA</span>
          </Button>

          <Button variant="secondary" size="md" onClick={() => setStaffOpen(true)}>
            <UserPlus className="w-4 h-4 text-[#2D2A6E]" strokeWidth={1.75} />
            <span>Create Staff ID</span>
          </Button>

          {/* ONE Primary CTA */}
          <Button variant="primary" size="md" onClick={() => setCreateOpen(true)}>
            <Plus className="w-4 h-4" strokeWidth={1.75} />
            <span>Create New Event</span>
          </Button>
        </div>
      </div>

      {/* Shareable Newly Created Staff Credentials Banner */}
      {issuedCredentials && (
        <div className="bg-white rounded-[14px] border border-[#0F766E]/35 shadow-card p-5 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <KeyRound className="w-4 h-4 text-[#0F766E]" strokeWidth={1.75} />
              <span className="text-[14px] font-semibold text-[#0F1419]">
                Staff Credentials Ready to Share ({issuedCredentials.name})
              </span>
              <span className="num px-2 py-0.5 rounded-[6px] bg-[#CCFBF1] text-[#0F766E] text-[11px] font-semibold">
                {issuedCredentials.role}
              </span>
            </div>
            <div className="num text-[13px] text-[#5C6570]">
              Login ID: <strong className="text-[#0F1419]">{issuedCredentials.email}</strong> ·
              Password: <strong className="text-[#0F1419]">{issuedCredentials.password}</strong>
            </div>
          </div>

          <div className="flex items-center gap-2 shrink-0">
            <Button
              variant="secondary"
              size="sm"
              onClick={() => {
                const text = `Venu Login\nID: ${issuedCredentials.email}\nPassword: ${issuedCredentials.password}`;
                if (navigator?.clipboard?.writeText) {
                  void navigator.clipboard.writeText(text);
                }
                onNotify('Staff ID and password copied to clipboard.', 'success');
              }}
            >
              <Copy className="w-3.5 h-3.5" strokeWidth={1.75} />
              <span>Copy Credentials</span>
            </Button>
            <button
              type="button"
              onClick={() => setIssuedCredentials(null)}
              className="p-1.5 rounded-[8px] text-[#5C6570] hover:text-[#0F1419] cursor-pointer"
              aria-label="Dismiss credentials banner"
            >
              <X className="w-4 h-4" strokeWidth={1.75} />
            </button>
          </div>
        </div>
      )}

      {/* 4-Column KPI Strip */}
      <section className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-5">
        <div className="bg-white rounded-[14px] border border-[#E7E5E0] p-5">
          <div className="text-[12px] font-medium text-[#5C6570] uppercase tracking-wider">
            Active Events
          </div>
          <div className="num text-[28px] font-semibold text-[#0F1419] mt-2">
            {stats?.activeEvents ?? 0}
          </div>
          <div className="text-[12px] text-[#5C6570] mt-1">On sale across Mumbai</div>
        </div>

        <div className="bg-white rounded-[14px] border border-[#E7E5E0] p-5">
          <div className="text-[12px] font-medium text-[#5C6570] uppercase tracking-wider">
            Seats Booked
          </div>
          <div className="num text-[28px] font-semibold text-[#0F1419] mt-2">
            {stats?.seatsBooked ?? 0}{' '}
            <span className="text-[16px] text-[#5C6570]">/ {stats?.totalSeatsCapacity ?? 0}</span>
          </div>
          <div className="text-[12px] text-[#0F766E] mt-1">Live pgx inventory</div>
        </div>

        <div className="bg-white rounded-[14px] border border-[#E7E5E0] p-5">
          <div className="text-[12px] font-medium text-[#5C6570] uppercase tracking-wider">
            Gross Ticket Revenue
          </div>
          <div className="num text-[28px] font-semibold text-[#0F1419] mt-2">
            {formatINR(stats?.grossTicketRevenue ?? 0)}
          </div>
          <div className="text-[12px] text-[#5C6570] mt-1">Razorpay verified</div>
        </div>

        <div className="bg-white rounded-[14px] border border-[#E7E5E0] p-5">
          <div className="text-[12px] font-medium text-[#5C6570] uppercase tracking-wider">
            Gate Admitted Today
          </div>
          <div className="num text-[28px] font-semibold text-[#0F1419] mt-2">
            {stats?.gateAdmittedToday ?? 0}
          </div>
          <div className="text-[12px] text-[#16A34A] mt-1">HMAC-SHA256 check-ins</div>
        </div>
      </section>

      {/* Event Catalog & Box Office Table */}
      <section className="bg-white rounded-[14px] border border-[#E7E5E0] shadow-card overflow-hidden">
        <div className="p-6 border-b border-[#E7E5E0] flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <h2 className="text-[18px] font-semibold text-[#0F1419]">
              Event Catalog &amp; Box Office
            </h2>
            <p className="text-[13px] text-[#5C6570]">
              Manage inventory status, inspect live seat maps, and track gross ticket revenue.
            </p>
          </div>

          <div className="relative w-full sm:w-64">
            <Search
              className="w-4 h-4 text-[#5C6570] absolute left-3 top-1/2 -translate-y-1/2"
              strokeWidth={1.75}
            />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search event or venue..."
              aria-label="Search events"
              className="w-full h-10 pl-9 pr-3 rounded-[8px] bg-[#FAFAF7] border border-[#E7E5E0] text-[13px] focus:outline-none focus:border-[#2D2A6E]"
            />
          </div>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="border-b border-[#E7E5E0] bg-[#FAFAF7] text-[12px] font-medium text-[#5C6570] uppercase tracking-wider">
                <th className="py-3.5 px-6">Event &amp; Venue</th>
                <th className="py-3.5 px-4">Date &amp; Schedule</th>
                <th className="py-3.5 px-4">Occupancy</th>
                <th className="py-3.5 px-4 text-right">Gross Revenue</th>
                <th className="py-3.5 px-4">Status</th>
                <th className="py-3.5 px-6 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#E7E5E0] text-[14px]">
              {filteredEvents.map((evt) => {
                const pct = Math.min(100, Math.round((evt.bookedSeats / evt.totalSeats) * 100));
                const isLive = evt.status === 'ON_SALE' || evt.status === 'ALMOST_FULL';
                return (
                  <tr key={evt.id} className="hover:bg-[#FAFAF7]/70 transition-colors">
                    <td className="py-4 px-6">
                      <div className="font-semibold text-[#0F1419]">{evt.title}</div>
                      <div className="text-[12px] text-[#5C6570] mt-0.5">
                        {evt.venueName} · {evt.category}
                      </div>
                    </td>
                    <td className="py-4 px-4 num text-[13px] text-[#0F1419] whitespace-nowrap">
                      {evt.dateLabel}
                      <div className="text-[12px] text-[#5C6570]">{evt.timeLabel}</div>
                    </td>
                    <td className="py-4 px-4">
                      <div className="num text-[13px] font-medium text-[#0F1419]">
                        {evt.bookedSeats} / {evt.totalSeats}{' '}
                        <span className="text-[#5C6570]">({pct}%)</span>
                      </div>
                      <div className="w-28 h-1.5 rounded-full bg-[#E7E5E0] mt-1.5 overflow-hidden">
                        <div
                          className="h-full bg-[#2D2A6E] rounded-full"
                          style={{ width: `${pct}%` }}
                        />
                      </div>
                    </td>
                    <td className="py-4 px-4 text-right num font-semibold text-[#0F1419] whitespace-nowrap">
                      {formatINR(evt.grossRevenue)}
                    </td>
                    <td className="py-4 px-4 whitespace-nowrap">
                      <span
                        className={`inline-flex items-center px-2.5 py-1 rounded-[6px] text-[11px] font-semibold num ${
                          isLive
                            ? 'bg-[#DCFCE7] text-[#16A34A]'
                            : 'bg-[#E7E5E0] text-[#5C6570]'
                        }`}
                      >
                        {evt.status.replace('_', ' ')}
                      </span>
                    </td>
                    <td className="py-4 px-6 text-right whitespace-nowrap space-x-2">
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => onInspectSeatMap(evt.id)}
                      >
                        <Eye className="w-3.5 h-3.5" strokeWidth={1.75} />
                        <span>Seat Map</span>
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={async () => {
                          const nextStatus = isLive ? 'PAUSED' : 'ON_SALE';
                          await updateStatus.mutateAsync({
                            eventId: evt.id,
                            status: nextStatus,
                          });
                          onNotify(`${evt.title} marked ${nextStatus}.`, 'info');
                        }}
                      >
                        {isLive ? 'Pause' : 'Publish'}
                      </Button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>

      {/* Staff Credentials Provisioned Directory */}
      <section className="bg-white rounded-[14px] border border-[#E7E5E0] shadow-card overflow-hidden">
        <div className="p-6 border-b border-[#E7E5E0] flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="flex items-start gap-3">
            <Users className="w-5 h-5 text-[#2D2A6E] mt-0.5" strokeWidth={1.75} />
            <div>
              <h2 className="text-[18px] font-semibold text-[#0F1419]">
                Venue Staff IDs ({staffUsers.length})
              </h2>
              <p className="text-[13px] text-[#5C6570]">
                Assigned credentials for gatekeepers and bartenders. Share login ID and password
                directly with staff.
              </p>
            </div>
          </div>

          <Button variant="secondary" size="sm" onClick={() => setStaffOpen(true)}>
            <UserPlus className="w-3.5 h-3.5" strokeWidth={1.75} />
            <span>Add Staff ID</span>
          </Button>
        </div>

        <div className="divide-y divide-[#E7E5E0]">
          {staffUsers.map((staff) => (
            <div
              key={staff.id}
              className="px-6 py-4 flex flex-wrap items-center justify-between gap-3 text-[14px]"
            >
              <div>
                <div className="font-semibold text-[#0F1419]">{staff.name}</div>
                <div className="num text-[13px] text-[#5C6570]">{staff.email}</div>
              </div>
              <span
                className={`num px-2.5 py-1 rounded-[6px] text-[11px] font-semibold ${
                  staff.role === 'SCANNER'
                    ? 'bg-[#FEF3C7] text-[#B45309]'
                    : 'bg-[#CCFBF1] text-[#0F766E]'
                }`}
              >
                {staff.role === 'SCANNER' ? 'GATEKEEPER (SCANNER)' : 'BARTENDER'}
              </span>
            </div>
          ))}
        </div>
      </section>

      {/* Origin-Aware Create Staff ID Modal */}
      <AnimatePresence>
        {staffOpen && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-[#0F1419]/40 backdrop-blur-[2px]"
            role="dialog"
            aria-modal="true"
            aria-labelledby="create-staff-title"
          >
            <motion.div
              initial={{ opacity: 0, scale: 0.94, y: -12 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.94, y: -12 }}
              transition={SPRING_SPATIAL}
              style={{ transformOrigin: 'top right' }}
              className="w-full max-w-md bg-white rounded-[20px] border border-[#E7E5E0] shadow-modal p-6"
            >
              <div className="flex items-center justify-between pb-4 border-b border-[#E7E5E0]">
                <h2 id="create-staff-title" className="text-[20px] font-semibold text-[#0F1419]">
                  Create Staff Login ID
                </h2>
                <button
                  type="button"
                  onClick={() => setStaffOpen(false)}
                  className="p-1.5 rounded-[8px] text-[#5C6570] hover:text-[#0F1419] cursor-pointer"
                  aria-label="Close staff modal"
                >
                  <X className="w-5 h-5" strokeWidth={1.75} />
                </button>
              </div>

              <form
                onSubmit={staffForm.handleSubmit(handleCreateStaffSubmit)}
                className="mt-5 space-y-4"
              >
                <div>
                  <label
                    htmlFor="staff-name"
                    className="block text-[13px] font-medium text-[#0F1419] mb-1"
                  >
                    Staff Full Name
                  </label>
                  <input
                    id="staff-name"
                    type="text"
                    placeholder="e.g. Kabir Gatekeeper"
                    {...staffForm.register('name')}
                    className="w-full h-11 px-3.5 rounded-[8px] border border-[#E7E5E0] text-[14px]"
                  />
                  {staffForm.formState.errors.name && (
                    <p className="text-[12px] text-[#DC2626] mt-1">
                      {staffForm.formState.errors.name.message}
                    </p>
                  )}
                </div>

                <div>
                  <label
                    htmlFor="staff-email"
                    className="block text-[13px] font-medium text-[#0F1419] mb-1"
                  >
                    Assigned Login ID / Email
                  </label>
                  <input
                    id="staff-email"
                    type="email"
                    placeholder="gate02@venu.live"
                    {...staffForm.register('email')}
                    className="w-full h-11 px-3.5 rounded-[8px] border border-[#E7E5E0] text-[14px]"
                  />
                  {staffForm.formState.errors.email && (
                    <p className="text-[12px] text-[#DC2626] mt-1">
                      {staffForm.formState.errors.email.message}
                    </p>
                  )}
                </div>

                <div>
                  <label
                    htmlFor="staff-password"
                    className="block text-[13px] font-medium text-[#0F1419] mb-1"
                  >
                    Initial Password
                  </label>
                  <input
                    id="staff-password"
                    type="text"
                    placeholder="Minimum 6 characters"
                    {...staffForm.register('password')}
                    className="w-full h-11 px-3.5 rounded-[8px] border border-[#E7E5E0] text-[14px] num"
                  />
                  {staffForm.formState.errors.password && (
                    <p className="text-[12px] text-[#DC2626] mt-1">
                      {staffForm.formState.errors.password.message}
                    </p>
                  )}
                </div>

                <div>
                  <label
                    htmlFor="staff-role"
                    className="block text-[13px] font-medium text-[#0F1419] mb-1"
                  >
                    Assigned Role
                  </label>
                  <select
                    id="staff-role"
                    {...staffForm.register('role')}
                    className="w-full h-11 px-3 rounded-[8px] border border-[#E7E5E0] text-[14px] bg-white"
                  >
                    <option value="SCANNER">Gatekeeper (SCANNER)</option>
                    <option value="BARTENDER">Bartender (BARTENDER)</option>
                  </select>
                </div>

                <div className="pt-2 flex justify-end gap-2">
                  <Button type="button" variant="ghost" onClick={() => setStaffOpen(false)}>
                    Cancel
                  </Button>
                  <Button type="submit" variant="primary" isLoading={createStaff.isPending}>
                    Provision Staff ID
                  </Button>
                </div>
              </form>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Origin-Aware Create Event Modal */}
      <AnimatePresence>
        {createOpen && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-[#0F1419]/40 backdrop-blur-[2px]"
            role="dialog"
            aria-modal="true"
            aria-labelledby="create-event-title"
          >
            <motion.div
              initial={{ opacity: 0, scale: 0.94, y: -12 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.94, y: -12 }}
              transition={SPRING_SPATIAL}
              style={{ transformOrigin: 'top right' }}
              className="w-full max-w-lg bg-white rounded-[20px] border border-[#E7E5E0] shadow-modal p-6 max-h-[90vh] overflow-y-auto"
            >
              <div className="flex items-center justify-between pb-4 border-b border-[#E7E5E0]">
                <h2 id="create-event-title" className="text-[20px] font-semibold text-[#0F1419]">
                  Create Live Event
                </h2>
                <button
                  type="button"
                  onClick={() => setCreateOpen(false)}
                  className="p-1.5 rounded-[8px] text-[#5C6570] hover:text-[#0F1419] cursor-pointer"
                  aria-label="Close modal"
                >
                  <X className="w-5 h-5" strokeWidth={1.75} />
                </button>
              </div>

              <form onSubmit={handleSubmit(handleCreateSubmit)} className="mt-5 space-y-4">
                <div>
                  <label className="block text-[13px] font-medium text-[#0F1419] mb-1">
                    Event Title
                  </label>
                  <input
                    type="text"
                    {...register('title')}
                    className="w-full h-11 px-3.5 rounded-[8px] border border-[#E7E5E0] text-[14px]"
                  />
                  {errors.title && (
                    <p className="text-[12px] text-[#DC2626] mt-1">{errors.title.message}</p>
                  )}
                </div>

                <div>
                  <label className="block text-[13px] font-medium text-[#0F1419] mb-1">
                    Editorial Subtitle
                  </label>
                  <input
                    type="text"
                    {...register('subtitle')}
                    className="w-full h-11 px-3.5 rounded-[8px] border border-[#E7E5E0] text-[14px]"
                  />
                  {errors.subtitle && (
                    <p className="text-[12px] text-[#DC2626] mt-1">{errors.subtitle.message}</p>
                  )}
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-[13px] font-medium text-[#0F1419] mb-1">
                      Category
                    </label>
                    <select
                      {...register('category')}
                      className="w-full h-11 px-3 rounded-[8px] border border-[#E7E5E0] text-[14px] bg-white"
                    >
                      <option value="Live Music">Live Music</option>
                      <option value="Stand-up Comedy">Stand-up Comedy</option>
                      <option value="Theater & Plays">Theater &amp; Plays</option>
                      <option value="Lounge Sessions">Lounge Sessions</option>
                    </select>
                  </div>

                  <div>
                    <label className="block text-[13px] font-medium text-[#0F1419] mb-1">
                      Floor Layout
                    </label>
                    <select
                      {...register('layoutType')}
                      className="w-full h-11 px-3 rounded-[8px] border border-[#E7E5E0] text-[14px] bg-white"
                    >
                      <option value="PROSCENIUM">Proscenium Hall</option>
                      <option value="CLUB">Club &amp; DJ Floor</option>
                    </select>
                  </div>
                </div>

                <div>
                  <label className="block text-[13px] font-medium text-[#0F1419] mb-1">
                    Venue Name
                  </label>
                  <input
                    type="text"
                    {...register('venueName')}
                    className="w-full h-11 px-3.5 rounded-[8px] border border-[#E7E5E0] text-[14px]"
                  />
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-[13px] font-medium text-[#0F1419] mb-1">
                      Date
                    </label>
                    <input
                      type="text"
                      {...register('dateLabel')}
                      className="w-full h-11 px-3.5 rounded-[8px] border border-[#E7E5E0] text-[14px] num"
                    />
                  </div>
                  <div>
                    <label className="block text-[13px] font-medium text-[#0F1419] mb-1">
                      Time
                    </label>
                    <input
                      type="text"
                      {...register('timeLabel')}
                      className="w-full h-11 px-3.5 rounded-[8px] border border-[#E7E5E0] text-[14px] num"
                    />
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-[13px] font-medium text-[#0F1419] mb-1">
                      VIP Price (₹)
                    </label>
                    <input
                      type="number"
                      {...register('vipPrice', { valueAsNumber: true })}
                      className="w-full h-11 px-3.5 rounded-[8px] border border-[#E7E5E0] text-[14px] num"
                    />
                  </div>
                  <div>
                    <label className="block text-[13px] font-medium text-[#0F1419] mb-1">
                      General Price (₹)
                    </label>
                    <input
                      type="number"
                      {...register('generalPrice', { valueAsNumber: true })}
                      className="w-full h-11 px-3.5 rounded-[8px] border border-[#E7E5E0] text-[14px] num"
                    />
                  </div>
                </div>

                <div className="pt-2 flex justify-end gap-2">
                  <Button type="button" variant="ghost" onClick={() => setCreateOpen(false)}>
                    Cancel
                  </Button>
                  <Button type="submit" variant="primary" isLoading={createEvent.isPending}>
                    Publish Event
                  </Button>
                </div>
              </form>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Origin-Aware Admin TOTP MFA Enrollment Modal */}
      <AnimatePresence>
        {mfaEnrollment && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-[#0F1419]/40 backdrop-blur-[2px]"
            role="dialog"
            aria-modal="true"
            aria-labelledby="mfa-enroll-title"
            onClick={() => setMfaEnrollment(null)}
          >
            <motion.div
              initial={{ opacity: 0, scale: 0.94, y: -12 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.94, y: -12 }}
              transition={SPRING_SPATIAL}
              style={{ transformOrigin: 'top right' }}
              onClick={(e) => e.stopPropagation()}
              className="w-full max-w-md bg-white rounded-[20px] border border-[#E7E5E0] shadow-modal p-6 space-y-5"
            >
              <div className="flex items-start justify-between gap-4 pb-4 border-b border-[#E7E5E0]">
                <div>
                  <div className="num text-[11px] uppercase tracking-wider text-[#2D2A6E] font-semibold">
                    RFC 6238 TOTP · Replay Protected
                  </div>
                  <h2 id="mfa-enroll-title" className="text-[20px] font-semibold text-[#0F1419] mt-1">
                    Authenticator App Setup
                  </h2>
                </div>
                <button
                  type="button"
                  onClick={() => setMfaEnrollment(null)}
                  className="p-1.5 rounded-[8px] text-[#5C6570] hover:text-[#0F1419] cursor-pointer"
                  aria-label="Close MFA modal"
                >
                  <X className="w-5 h-5" strokeWidth={1.75} />
                </button>
              </div>

              <div className="flex flex-col items-center p-5 rounded-[14px] bg-[#FAFAF7] border border-[#E7E5E0] space-y-3">
                <QRCodeSVG
                  value={mfaEnrollment.otpauthUri}
                  size={180}
                  bgColor="#FAFAF7"
                  fgColor="#0F1419"
                />
                <div className="num text-[11px] text-[#5C6570]">
                  {mfaEnrollment.issuer} · {mfaEnrollment.algorithm} · {mfaEnrollment.digits} digits ·{' '}
                  {mfaEnrollment.period}s
                </div>
              </div>

              <div className="space-y-1.5">
                <div className="text-[12px] font-medium text-[#5C6570]">
                  Manual Setup Key (160-bit Base32)
                </div>
                <div className="flex items-center justify-between gap-2 p-3 rounded-[10px] bg-[#FAFAF7] border border-[#E7E5E0]">
                  <code className="num text-[12px] font-semibold text-[#0F1419] break-all">
                    {mfaEnrollment.secret}
                  </code>
                  <button
                    type="button"
                    onClick={() => {
                      if (navigator?.clipboard?.writeText) {
                        void navigator.clipboard.writeText(mfaEnrollment.secret);
                      }
                      setCopiedSecret(true);
                      onNotify('TOTP secret copied to clipboard.', 'info');
                    }}
                    className="p-1.5 rounded-[6px] bg-white border border-[#E7E5E0] text-[#0F1419] hover:bg-[#F4F3F0] cursor-pointer active:scale-95 shrink-0"
                    aria-label="Copy Base32 secret"
                  >
                    {copiedSecret ? (
                      <Check className="w-4 h-4 text-[#16A34A]" strokeWidth={1.75} />
                    ) : (
                      <Copy className="w-4 h-4" strokeWidth={1.75} />
                    )}
                  </button>
                </div>
                <p className="text-[12px] text-[#5C6570] leading-relaxed">
                  Your secret is encrypted with AES-256-GCM at rest. Each 6-digit code is strictly
                  single-use per 30-second window.
                </p>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>
    </div>
  );
};
