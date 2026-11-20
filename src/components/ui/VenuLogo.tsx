import React from 'react';

interface VenuLogoProps {
  onClick?: () => void;
}

export const VenuLogo: React.FC<VenuLogoProps> = ({ onClick }) => {
  return (
    <button
      type="button"
      onClick={onClick}
      className="inline-flex items-center gap-2.5 text-left group whitespace-nowrap shrink-0 cursor-pointer active:scale-95 transition-all duration-200 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-[#2D2A6E] rounded-md"
      aria-label="Venu Home"
    >
      <span className="inline-flex items-center justify-center w-8 h-8 rounded-[8px] bg-[#2D2A6E] text-white shrink-0">
        <svg
          width="18"
          height="18"
          viewBox="0 0 20 20"
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
          aria-hidden="true"
        >
          <path
            d="M3 6.5C3 5.67157 3.67157 5 4.5 5H15.5C16.3284 5 17 5.67157 17 6.5V8.25C16.0335 8.25 15.25 9.0335 15.25 10C15.25 10.9665 16.0335 11.75 17 11.75V13.5C17 14.3284 16.3284 15 15.5 15H4.5C3.67157 15 3 14.3284 3 13.5V11.75C3.9665 11.75 4.75 10.9665 4.75 10C4.75 9.0335 3.9665 8.25 3 8.25V6.5Z"
            stroke="currentColor"
            strokeWidth="1.75"
            strokeLinejoin="round"
          />
          <path
            d="M7.5 10L9.5 12L12.75 8.25"
            stroke="currentColor"
            strokeWidth="1.75"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </span>
      <span className="font-[Space_Grotesk] text-[20px] font-semibold tracking-[-0.02em] text-[#0F1419]">
        Venu
      </span>
    </button>
  );
};
