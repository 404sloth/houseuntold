import React from 'react';
import { AnimatePresence, motion } from 'motion/react';
import { AlertCircle, CheckCircle2, Info, X } from 'lucide-react';
import { SPRING_SPATIAL } from '../../lib/motion/variants';

export interface ToastMessage {
  id: string;
  title: string;
  tone: 'error' | 'success' | 'info';
}

interface ToastContainerProps {
  toasts: ToastMessage[];
  onDismiss: (id: string) => void;
}

export const ToastContainer: React.FC<ToastContainerProps> = ({ toasts, onDismiss }) => {
  return (
    <div
      className="fixed bottom-5 right-5 z-50 flex flex-col gap-2.5 max-w-sm w-full pointer-events-none px-4 sm:px-0"
      role="region"
      aria-label="Notifications"
    >
      <AnimatePresence mode="popLayout">
        {toasts.map((toast) => {
          const isError = toast.tone === 'error';
          const isSuccess = toast.tone === 'success';
          return (
            <motion.div
              key={toast.id}
              layout
              initial={{ opacity: 0, y: 16, scale: 0.96 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: 12, scale: 0.96 }}
              transition={SPRING_SPATIAL}
              role="status"
              className={`pointer-events-auto flex items-start gap-3 rounded-[12px] bg-white border p-3.5 shadow-modal ${
                isError
                  ? 'border-[#DC2626]/30 text-[#0F1419]'
                  : isSuccess
                    ? 'border-[#16A34A]/30 text-[#0F1419]'
                    : 'border-[#E7E5E0] text-[#0F1419]'
              }`}
            >
              {isError ? (
                <AlertCircle className="w-5 h-5 text-[#DC2626] shrink-0 mt-0.5" strokeWidth={1.75} />
              ) : isSuccess ? (
                <CheckCircle2 className="w-5 h-5 text-[#16A34A] shrink-0 mt-0.5" strokeWidth={1.75} />
              ) : (
                <Info className="w-5 h-5 text-[#2D2A6E] shrink-0 mt-0.5" strokeWidth={1.75} />
              )}
              <p className="text-[13px] font-medium leading-5 flex-1">{toast.title}</p>
              <button
                type="button"
                onClick={() => onDismiss(toast.id)}
                className="p-1 rounded-md text-[#5C6570] hover:text-[#0F1419] hover:bg-[#FAFAF7] cursor-pointer active:scale-95 transition-all"
                aria-label="Dismiss notification"
              >
                <X className="w-4 h-4" strokeWidth={1.75} />
              </button>
            </motion.div>
          );
        })}
      </AnimatePresence>
    </div>
  );
};
