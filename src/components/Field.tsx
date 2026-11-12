import React from 'react';

export interface FieldProps {
  label?: React.ReactNode;
  helperText?: string;
  error?: string;
  children: React.ReactNode;
  className?: string;
  id?: string;
}

export const Field: React.FC<FieldProps> = ({
  label,
  helperText,
  error,
  children,
  className = '',
  id,
}) => {
  return (
    <div className={`flex flex-col ${className}`}>
      {label && (
        <label
          htmlFor={id}
          className="text-[13px] font-medium text-text mb-[6px] select-none"
        >
          {label}
        </label>
      )}
      {children}
      {error ? (
        <span className="text-[12px] text-danger mt-[6px]">
          {error}
        </span>
      ) : helperText ? (
        <span className="text-[12px] text-text-3 mt-[6px]">
          {helperText}
        </span>
      ) : null}
    </div>
  );
};

