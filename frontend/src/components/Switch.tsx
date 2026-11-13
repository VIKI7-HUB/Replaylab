import React from 'react';
import * as SwitchPrimitive from '@radix-ui/react-switch';

export interface SwitchProps extends React.ComponentPropsWithoutRef<typeof SwitchPrimitive.Root> {
  label?: string;
}

export const Switch = React.forwardRef<
  React.ElementRef<typeof SwitchPrimitive.Root>,
  SwitchProps
>(({ className = '', label, id, ...props }, ref) => {
  return (
    <div className="inline-flex items-center gap-2">
      <SwitchPrimitive.Root
        ref={ref}
        id={id}
        className={`relative inline-flex h-[18px] w-[32px] shrink-0 cursor-pointer items-center rounded-md transition-colors duration-150 data-[state=unchecked]:bg-bg-muted data-[state=unchecked]:border data-[state=unchecked]:border-border-strong data-[state=checked]:bg-text ${className}`}
        {...props}
      >
        <SwitchPrimitive.Thumb className="pointer-events-none block h-[14px] w-[14px] rounded bg-bg shadow-none transition-transform duration-150 data-[state=checked]:translate-x-[15px] data-[state=unchecked]:translate-x-[1px]" />
      </SwitchPrimitive.Root>
      {label && (
        <label htmlFor={id} className="text-[13px] text-text select-none cursor-pointer">
          {label}
        </label>
      )}
    </div>
  );
});

Switch.displayName = 'Switch';

