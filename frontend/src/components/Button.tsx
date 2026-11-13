import React from 'react';

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
  size?: 'sm' | 'md';
  loading?: boolean;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ variant = 'secondary', size = 'md', loading = false, disabled, children, className = '', ...props }, ref) => {
    const sizeClasses = size === 'sm' ? 'h-7 px-3 text-[13px] font-medium' : 'h-8 px-3.5 text-[13px] font-medium';

    let variantClasses = '';
    if (variant === 'primary') {
      variantClasses = 'btn-primary';
    } else if (variant === 'secondary') {
      variantClasses = 'bg-bg-subtle border border-border-strong text-text hover:bg-bg-muted hover:border-accent';
    } else if (variant === 'ghost') {
      variantClasses = 'bg-transparent text-text-2 hover:bg-bg-muted hover:text-text';
    } else if (variant === 'danger') {
      variantClasses = 'bg-transparent border border-border-strong text-danger hover:bg-danger-bg hover:border-danger';
    }

    const disabledClasses = disabled || loading ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer';

    return (
      <button
        ref={ref}
        disabled={disabled || loading}
        className={`inline-flex items-center justify-center rounded transition-colors duration-150 select-none ${sizeClasses} ${variantClasses} ${disabledClasses} ${className}`}
        {...props}
      >
        {loading ? 'Working…' : children}
      </button>
    );
  }
);

Button.displayName = 'Button';

