import React from 'react';
import * as SelectPrimitive from '@radix-ui/react-select';
import { ChevronDownIcon, CheckIcon } from '@radix-ui/react-icons';

export interface SelectOption {
  value: string;
  label: string;
}

export interface SelectProps {
  value?: string;
  onValueChange?: (val: string) => void;
  options: SelectOption[];
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}

const EMPTY_VALUE_SENTINEL = '__EMPTY_SENTINEL__';

export const Select: React.FC<SelectProps> = ({
  value,
  onValueChange,
  options,
  placeholder = 'Select…',
  disabled,
  className = '',
}) => {
  const internalValue = value === '' ? EMPTY_VALUE_SENTINEL : value;

  const handleValueChange = (val: string) => {
    if (onValueChange) {
      onValueChange(val === EMPTY_VALUE_SENTINEL ? '' : val);
    }
  };

  return (
    <SelectPrimitive.Root
      value={internalValue}
      onValueChange={handleValueChange}
      disabled={disabled}
    >
      <SelectPrimitive.Trigger
        className={`h-8 w-full inline-flex items-center justify-between px-2.5 text-[13px] text-text bg-bg border border-border-strong rounded transition-colors placeholder:text-text-3 disabled:opacity-50 disabled:cursor-not-allowed ${className}`}
      >
        <SelectPrimitive.Value placeholder={placeholder} />
        <SelectPrimitive.Icon asChild>
          <ChevronDownIcon className="h-[15px] w-[15px] text-text-2 shrink-0" />
        </SelectPrimitive.Icon>
      </SelectPrimitive.Trigger>

      <SelectPrimitive.Portal>
        <SelectPrimitive.Content
          className="z-50 min-w-[var(--radix-select-trigger-width)] max-h-[min(320px,var(--radix-select-content-available-height))] overflow-hidden rounded bg-bg text-text shadow-float border border-border animate-fade-in"
          position="popper"
          sideOffset={4}
        >
          <SelectPrimitive.Viewport className="p-1 max-h-[inherit] overflow-y-auto">
            {options.map((opt) => {
              const itemValue = opt.value === '' ? EMPTY_VALUE_SENTINEL : opt.value;
              return (
                <SelectPrimitive.Item
                  key={itemValue}
                  value={itemValue}
                  className="relative flex cursor-pointer select-none items-center rounded-sm py-1.5 pl-2 pr-6 text-[13px] text-text outline-none data-[highlighted]:bg-bg-muted data-[highlighted]:text-text"
                >
                  <SelectPrimitive.ItemText>{opt.label}</SelectPrimitive.ItemText>
                  <SelectPrimitive.ItemIndicator className="absolute right-2 inline-flex items-center">
                    <CheckIcon className="h-3.5 w-3.5" />
                  </SelectPrimitive.ItemIndicator>
                </SelectPrimitive.Item>
              );
            })}
          </SelectPrimitive.Viewport>
        </SelectPrimitive.Content>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  );
};
