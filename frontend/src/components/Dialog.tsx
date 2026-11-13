import React from 'react';
import * as DialogPrimitive from '@radix-ui/react-dialog';
import { Button } from './Button';

export interface DialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  confirmVariant?: 'primary' | 'secondary' | 'danger';
  onConfirm: () => void;
  loading?: boolean;
  children?: React.ReactNode;
}

export const Dialog: React.FC<DialogProps> = ({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  confirmVariant = 'primary',
  onConfirm,
  loading = false,
  children,
}) => {
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-[var(--overlay)] data-[state=open]:animate-fade-in data-[state=closed]:animate-fade-out" />
        <DialogPrimitive.Content className="fixed top-1/2 left-1/2 z-50 w-full max-w-[440px] -translate-x-1/2 -translate-y-1/2 rounded-md bg-bg p-5 shadow-float border border-border focus:outline-none data-[state=open]:animate-fade-in data-[state=closed]:animate-fade-out">
          <DialogPrimitive.Title className="section-title text-text mb-2">
            {title}
          </DialogPrimitive.Title>
          {description && (
            <DialogPrimitive.Description className="text-[14px] text-text-2 mb-4">
              {description}
            </DialogPrimitive.Description>
          )}
          {children && <div className="mb-4">{children}</div>}
          <div className="flex items-center justify-end gap-2 mt-5">
            <Button
              size="md"
              variant="secondary"
              onClick={() => onOpenChange(false)}
              disabled={loading}
            >
              {cancelLabel}
            </Button>
            <Button
              size="md"
              variant={confirmVariant}
              onClick={onConfirm}
              loading={loading}
            >
              {confirmLabel}
            </Button>
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
};

