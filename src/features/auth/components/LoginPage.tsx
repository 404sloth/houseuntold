import React, { useState } from 'react';
import { motion } from 'motion/react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { ArrowLeft, KeyRound, Mail } from 'lucide-react';
import {
  AdminLoginFormSchema,
  GoogleAuthInputSchema,
  type AdminLoginForm,
  type GoogleAuthInput,
  type UserRole,
} from '../../../lib/api/schemas';
import { SPRING_SPATIAL } from '../../../lib/motion/variants';
import { Button } from '../../../components/ui/Button';
import { VenuLogo } from '../../../components/ui/VenuLogo';
import { useAuth } from '../hooks/useAuth';

interface LoginPageProps {
  onBackHome: () => void;
  onAuthenticated: (role: UserRole) => void;
  onRequireMfa: (mfaChallengeToken: string, email: string) => void;
  onNotify: (title: string, tone: 'success' | 'error' | 'info') => void;
}

export const LoginPage: React.FC<LoginPageProps> = ({
  onBackHome,
  onAuthenticated,
  onRequireMfa,
  onNotify,
}) => {
  const { loginGoogle, loginStaffOrAdmin } = useAuth();
  const [googleEmailOpen, setGoogleEmailOpen] = useState(false);

  const googleForm = useForm<GoogleAuthInput>({
    resolver: zodResolver(GoogleAuthInputSchema),
    defaultValues: {
      name: 'Arjun Mehta',
      email: 'arjun.mehta@gmail.com',
    },
  });

  const credentialsForm = useForm<AdminLoginForm>({
    resolver: zodResolver(AdminLoginFormSchema),
    defaultValues: {
      email: '',
      password: '',
    },
  });

  const handleGoogleSubmit = async (values: GoogleAuthInput) => {
    try {
      const session = await loginGoogle.mutateAsync(values);
      onNotify(`Signed in as ${session.user.name}`, 'success');
      onAuthenticated(session.user.role);
    } catch (err) {
      onNotify(err instanceof Error ? err.message : 'Google sign-in failed', 'error');
    }
  };

  const handleCredentialsSubmit = async (values: AdminLoginForm) => {
    try {
      const res = await loginStaffOrAdmin.mutateAsync(values);
      if (res.requires_mfa) {
        onNotify('Credentials verified. Enter 6-digit authenticator code.', 'info');
        onRequireMfa(res.mfa_challenge_token, values.email);
        return;
      }
      onNotify(`Signed in · ${res.user.name}`, 'success');
      onAuthenticated(res.user.role);
    } catch (err) {
      onNotify(err instanceof Error ? err.message : 'Invalid credentials', 'error');
    }
  };

  return (
    <div className="max-w-[460px] mx-auto px-5 py-12 sm:py-16">
      <button
        type="button"
        onClick={onBackHome}
        className="inline-flex items-center gap-1.5 text-[13px] font-medium text-[#5C6570] hover:text-[#0F1419] mb-6 cursor-pointer active:scale-95 transition-all"
      >
        <ArrowLeft className="w-4 h-4" strokeWidth={1.75} />
        <span>Back to Home</span>
      </button>

      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={SPRING_SPATIAL}
        className="bg-white rounded-[14px] border border-[#E7E5E0] shadow-editorial p-7 sm:p-8 space-y-6"
      >
        <div className="space-y-2">
          <VenuLogo />
          <h1 className="text-[26px] font-semibold tracking-tight text-[#0F1419] pt-2">
            Sign in to Venu
          </h1>
          <p className="text-[14px] text-[#5C6570]">
            Continue with Google or sign in with your assigned ID and password.
          </p>
        </div>

        {/* Google OAuth Sign-In */}
        <div className="space-y-3">
          {!googleEmailOpen ? (
            <Button
              type="button"
              variant="secondary"
              size="lg"
              className="w-full"
              isLoading={loginGoogle.isPending}
              onClick={() => handleGoogleSubmit(googleForm.getValues())}
            >
              <svg className="w-4 h-4" viewBox="0 0 24 24" aria-hidden="true">
                <path
                  fill="#4285F4"
                  d="M23.745 12.27c0-.7-.06-1.4-.19-2.07H12v4.51h6.6c-.29 1.52-1.14 2.82-2.4 3.68v3.05h3.88c2.27-2.09 3.665-5.17 3.665-9.17Z"
                />
                <path
                  fill="#34A853"
                  d="M12 24c3.24 0 5.95-1.08 7.93-2.91l-3.88-3.05c-1.08.72-2.45 1.16-4.05 1.16-3.12 0-5.77-2.11-6.72-4.96H1.29v3.14C3.26 21.3 7.31 24 12 24Z"
                />
                <path
                  fill="#FBBC05"
                  d="M5.28 14.24c-.24-.72-.38-1.49-.38-2.24s.14-1.52.38-2.24V6.62H1.29C.47 8.24 0 10.06 0 12s.47 3.76 1.29 5.38l3.99-3.14Z"
                />
                <path
                  fill="#EA4335"
                  d="M12 4.75c1.77 0 3.35.61 4.6 1.8l3.42-3.42C17.95 1.19 15.24 0 12 0 7.31 0 3.26 2.7 1.29 6.62l3.99 3.14c.95-2.85 3.6-4.96 6.72-4.96Z"
                />
              </svg>
              <span>Continue with Google</span>
            </Button>
          ) : (
            <form onSubmit={googleForm.handleSubmit(handleGoogleSubmit)} className="space-y-3">
              <div>
                <label
                  htmlFor="google-email"
                  className="block text-[13px] font-medium text-[#0F1419] mb-1"
                >
                  Google Email
                </label>
                <input
                  id="google-email"
                  type="email"
                  {...googleForm.register('email')}
                  className="w-full h-11 px-3.5 rounded-[8px] bg-white border border-[#E7E5E0] text-[14px] focus:outline-none focus:border-[#2D2A6E] focus:ring-[3px] focus:ring-[#2D2A6E]/12"
                />
              </div>
              <Button
                type="submit"
                variant="secondary"
                size="lg"
                className="w-full"
                isLoading={loginGoogle.isPending}
              >
                <span>Continue with Google</span>
              </Button>
            </form>
          )}

          <button
            type="button"
            onClick={() => setGoogleEmailOpen((prev) => !prev)}
            className="w-full text-center text-[12px] text-[#5C6570] hover:text-[#0F1419] cursor-pointer transition-colors"
          >
            {googleEmailOpen ? 'Use default Google account' : 'Use a different Google account'}
          </button>
        </div>

        <div className="relative flex items-center py-1">
          <div className="flex-grow border-t border-[#E7E5E0]" />
          <span className="shrink mx-3 text-[11px] uppercase tracking-wider text-[#5C6570]">
            Or Sign In with ID &amp; Password
          </span>
          <div className="flex-grow border-t border-[#E7E5E0]" />
        </div>

        {/* Role-Blind Credentials Form — Backend determines role */}
        <form onSubmit={credentialsForm.handleSubmit(handleCredentialsSubmit)} className="space-y-4">
          <div>
            <label
              htmlFor="login-email"
              className="block text-[13px] font-medium text-[#0F1419] mb-1.5"
            >
              Email / Login ID
            </label>
            <div className="relative">
              <Mail
                className="w-4 h-4 text-[#5C6570] absolute left-3.5 top-1/2 -translate-y-1/2"
                strokeWidth={1.75}
              />
              <input
                id="login-email"
                type="email"
                autoComplete="username"
                placeholder="name@venu.live"
                {...credentialsForm.register('email')}
                className="w-full h-11 pl-10 pr-3.5 rounded-[8px] bg-white border border-[#E7E5E0] text-[14px] text-[#0F1419] focus:outline-none focus:border-[#2D2A6E] focus:ring-[3px] focus:ring-[#2D2A6E]/12"
              />
            </div>
            {credentialsForm.formState.errors.email && (
              <p className="text-[12px] text-[#DC2626] mt-1">
                {credentialsForm.formState.errors.email.message}
              </p>
            )}
          </div>

          <div>
            <label
              htmlFor="login-password"
              className="block text-[13px] font-medium text-[#0F1419] mb-1.5"
            >
              Password
            </label>
            <div className="relative">
              <KeyRound
                className="w-4 h-4 text-[#5C6570] absolute left-3.5 top-1/2 -translate-y-1/2"
                strokeWidth={1.75}
              />
              <input
                id="login-password"
                type="password"
                autoComplete="current-password"
                placeholder="Enter password"
                {...credentialsForm.register('password')}
                className="w-full h-11 pl-10 pr-3.5 rounded-[8px] bg-white border border-[#E7E5E0] text-[14px] text-[#0F1419] focus:outline-none focus:border-[#2D2A6E] focus:ring-[3px] focus:ring-[#2D2A6E]/12"
              />
            </div>
            {credentialsForm.formState.errors.password && (
              <p className="text-[12px] text-[#DC2626] mt-1">
                {credentialsForm.formState.errors.password.message}
              </p>
            )}
          </div>

          <Button
            type="submit"
            variant="primary"
            size="lg"
            className="w-full"
            isLoading={loginStaffOrAdmin.isPending}
          >
            <span>Sign In</span>
          </Button>
        </form>
      </motion.div>
    </div>
  );
};
