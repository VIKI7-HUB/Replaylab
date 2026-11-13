import React from 'react';

export interface FreshnessMeterProps {
  score: number;
  showNumber?: boolean;
  className?: string;
}

export const FreshnessMeter: React.FC<FreshnessMeterProps> = ({
  score,
  showNumber = true,
  className = '',
}) => {
  const clamped = Math.max(0, Math.min(100, Math.round(score)));

  let fillColor = 'meter-danger';
  if (clamped >= 70) {
    fillColor = 'meter-success';
  } else if (clamped >= 40) {
    fillColor = 'meter-warning';
  }

  return (
    <div className={`inline-flex items-center gap-2 ${className}`}>
      <div className="w-20 h-1.5 bg-bg-muted rounded-full overflow-hidden shrink-0">
        <div
          className={`h-full ${fillColor} rounded-full transition-all duration-300`}
          style={{ width: `${clamped}%` }}
        />
      </div>
      {showNumber && (
        <span className="text-[12px] text-text font-medium tabular-nums">
          {score.toFixed(1)}
        </span>
      )}
    </div>
  );
};

