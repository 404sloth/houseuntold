import React, { useLayoutEffect, useRef, useState } from 'react';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
export type ButtonSize = 'sm' | 'md' | 'lg';

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  isLoading?: boolean;
  children: React.ReactNode;
}

const VARIANT_CLASSES: Record<ButtonVariant, string> = {
  primary:
    'bg-[#2D2A6E] text-white hover:bg-[#3B3791] border border-transparent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#2D2A6E]',
  secondary:
    'bg-white text-[#0F1419] border border-[#E7E5E0] hover:bg-[#F4F3F0] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#2D2A6E]',
  ghost:
    'bg-transparent text-[#0F1419] border border-transparent hover:bg-[#F4F3F0] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#2D2A6E]',
  danger:
    'bg-transparent text-[#DC2626] border border-[#E7E5E0] hover:bg-[#FEE2E2] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#DC2626]',
};

const SIZE_CLASSES: Record<ButtonSize, string> = {
  sm: 'h-9 px-3.5 text-[13px]',
  md: 'h-11 px-5 text-[14px]',
  lg: 'h-12 px-6 text-[15px]',
};

export const Button: React.FC<ButtonProps> = ({
  variant = 'primary',
  size = 'md',
  isLoading = false,
  disabled,
  className = '',
  children,
  style,
  ...rest
}) => {
  const btnRef = useRef<HTMLButtonElement | null>(null);
  const [lockedWidth, setLockedWidth] = useState<number | undefined>(undefined);

  useLayoutEffect(() => {
    if (isLoading && btnRef.current && lockedWidth === undefined) {
      setLockedWidth(btnRef.current.getBoundingClientRect().width);
    } else if (!isLoading && lockedWidth !== undefined) {
      setLockedWidth(undefined);
    }
  }, [isLoading, lockedWidth]);

  return (
    <button
      ref={btnRef}
      disabled={disabled || isLoading}
      style={lockedWidth ? { ...style, width: `${lockedWidth}px` } : style}
      className={`inline-flex items-center justify-center gap-2 rounded-[8px] font-medium whitespace-nowrap shrink-0 cursor-pointer select-none active:scale-95 transition-all duration-200 disabled:opacity-55 disabled:cursor-not-allowed disabled:active:scale-100 ${VARIANT_CLASSES[variant]} ${SIZE_CLASSES[size]} ${className}`}
      {...rest}
    >
      {isLoading ? (
        <span className="inline-flex items-center gap-1" aria-label="Loading">
          <span className="w-1.5 h-1.5 rounded-full bg-current animate-bounce [animation-delay:-0.24s]" />
          <span className="w-1.5 h-1.5 rounded-full bg-current animate-bounce [animation-delay:-0.12s]" />
          <span className="w-1.5 h-1.5 rounded-full bg-current animate-bounce" />
        </span>
      ) : (
        children
      )}
    </button>
  );
};
