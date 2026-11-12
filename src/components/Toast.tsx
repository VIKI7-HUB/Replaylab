import React, { createContext, useContext, useState, useCallback } from 'react';
import * as ToastPrimitive from '@radix-ui/react-toast';

export type ToastTone = 'success' | 'warning' | 'danger' | 'neutral';

export interface ToastMessage {
  id: string;
  title?: string;
  description: string;
  tone?: ToastTone;
}

interface ToastContextType {
  toast: (description: string, tone?: ToastTone, title?: string) => void;
}

const ToastContext = createContext<ToastContextType | undefined>(undefined);

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const toast = useCallback((description: string, tone: ToastTone = 'neutral', title?: string) => {
    const id = Math.random().toString(36).substring(2, 9);
    setToasts((prev) => {
      // Keep at most 3 toasts
      const next = [...prev, { id, description, tone, title }];
      return next.slice(-3);
    });
  }, []);

  const removeToast = (id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  };

  return (
    <ToastContext.Provider value={{ toast }}>
      <ToastPrimitive.Provider duration={5000} swipeDirection="right">
        {children}
        {toasts.map((t) => {
          const borderToneColor = {
            success: 'border-l-success',
            warning: 'border-l-warning',
            danger: 'border-l-danger',
            neutral: 'border-l-border-strong',
          }[t.tone || 'neutral'];

          return (
            <ToastPrimitive.Root
              key={t.id}
              onOpenChange={(open) => {
                if (!open) removeToast(t.id);
              }}
              className={`fixed bottom-4 right-4 z-50 w-[360px] bg-bg p-3 shadow-float rounded border border-border border-l-[3px] ${borderToneColor} text-[13px] text-text transition-all data-[state=open]:animate-in data-[state=closed]:animate-out data-[swipe=end]:animate-out`}
            >
              {t.title && (
                <ToastPrimitive.Title className="font-medium text-text mb-0.5">
                  {t.title}
                </ToastPrimitive.Title>
              )}
              <ToastPrimitive.Description className="text-text-2">
                {t.description}
              </ToastPrimitive.Description>
            </ToastPrimitive.Root>
          );
        })}
        <ToastPrimitive.Viewport className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 w-[360px] max-w-[100vw] pointer-events-none" />
      </ToastPrimitive.Provider>
    </ToastContext.Provider>
  );
};

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used within ToastProvider');
  return ctx;
}

