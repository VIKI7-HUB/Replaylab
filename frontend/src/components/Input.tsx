import React from 'react';

export interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  unit?: string;
  hasError?: boolean;
}

export const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ unit, hasError, className = '', ...props }, ref) => {
    return (
      <div className="relative inline-flex items-center w-full">
        <input
          ref={ref}
          className={`h-8 w-full px-2.5 text-[13px] text-text bg-bg border ${
            hasError ? 'border-danger' : 'border-border-strong'
          } rounded placeholder:text-text-3 transition-colors ${
            unit ? 'pr-7' : ''
          } ${className}`}
          {...props}
        />
        {unit && (
          <span className="absolute right-2.5 text-[12px] text-text-3 pointer-events-none select-none">
            {unit}
          </span>
        )}
      </div>
    );
  }
);
Input.displayName = 'Input';

export interface TextareaProps extends React.TextareaHTMLAttributes<HTMLTextAreaElement> {
  hasError?: boolean;
}

export const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ hasError, className = '', ...props }, ref) => {
    return (
      <textarea
        ref={ref}
        className={`w-full px-2.5 py-2 text-[13px] text-text bg-bg border ${
          hasError ? 'border-danger' : 'border-border-strong'
        } rounded placeholder:text-text-3 transition-colors ${className}`}
        {...props}
      />
    );
  }
);
Textarea.displayName = 'Textarea';

