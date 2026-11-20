export type {
  UserRole,
  AuthProvider,
  AuthSession,
  GoogleAuthInput,
  FirebasePhoneAuthInput,
  TierType,
  Gender,
  SeatStatus,
  EventStatus,
  LayoutType,
  User,
  StaffUserSummary,
  CreateStaffUserInput,
  EventSection,
  VenuEvent,
  Seat,
  ServerOrderBreakdown,
  SeatMapResponse,
  AttendeeInput,
  AttendeeDetailsForm,
  TicketAttendee,
  Ticket,
  CheckoutInitResponse,
  PaymentMethod,
  CheckoutConfirmRequest,
  FnbMenuItem,
  AddFnbMenuItemInput,
  FnbTransaction,
  FnbQuoteRequest,
  FnbQuoteResponse,
  FnbSettleRequest,
  FnbSettleResponse,
  RecentScan,
  CreateEventForm,
  AdminLoginForm,
  AdminMfaInput,
  AdminMfaEnrollmentResponse,
  StaffAdminLoginResponse,
  TicketQrResponse,
} from './schemas';

export interface AdminOverviewStats {
  activeEvents: number;
  seatsBooked: number;
  totalSeatsCapacity: number;
  grossTicketRevenue: number;
  gateAdmittedToday: number;
}

export interface ScanLookupResult {
  valid: boolean;
  reason:
    | 'VALID'
    | 'ALREADY_CHECKED_IN'
    | 'INVALID_SIGNATURE'
    | 'EXPIRED'
    | 'ROTATING_QR_EXPIRED'
    | 'NOT_FOUND';
  ticket: import('./schemas').Ticket | null;
}
