import React from 'react';
import * as CheckboxPrimitive from '@radix-ui/react-checkbox';
import { CheckIcon } from '@radix-ui/react-icons';

export interface CheckboxProps extends React.ComponentPropsWithoutRef<typeof CheckboxPrimitive.Root> {
  label?: React.ReactNode;
}

export const Checkbox = React.forwardRef<
  React.ElementRef<typeof CheckboxPrimitive.Root>,
  CheckboxProps
>(({ className = '', label, id, ...props }, ref) => {
  return (
    <div className="inline-flex items-center gap-2">
      <CheckboxPrimitive.Root
        ref={ref}
        id={id}
        className={`h-[14px] w-[14px] shrink-0 rounded-sm border border-border-strong bg-bg flex items-center justify-center transition-colors data-[state=checked]:bg-text data-[state=checked]:border-text ${className}`}
        {...props}
      >
        <CheckboxPrimitive.Indicator className="flex items-center justify-center text-bg">
          <CheckIcon className="h-3 w-3" />
        </CheckboxPrimitive.Indicator>
      </CheckboxPrimitive.Root>
      {label && (
        <label htmlFor={id} className="text-[13px] text-text select-none cursor-pointer">
          {label}
        </label>
      )}
    </div>
  );
});

Checkbox.displayName = 'Checkbox';

