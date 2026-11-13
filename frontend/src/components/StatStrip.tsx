import React from 'react';

export interface StatCell {
  label: string;
  value: React.ReactNode;
  detail?: string;
}

export interface StatStripProps {
  cells: StatCell[];
  className?: string;
}

export const StatStrip: React.FC<StatStripProps> = ({ cells, className = '' }) => {
  // Show at most 4 cells per strip.
  const displayCells = cells.slice(0, 4);

  return (
    <div
      className={`bg-bg border border-border rounded-md grid divide-x divide-border ${
        displayCells.length === 1
          ? 'grid-cols-1'
          : displayCells.length === 2
          ? 'grid-cols-2'
          : displayCells.length === 3
          ? 'grid-cols-3'
          : 'grid-cols-4'
      } ${className}`}
    >
      {displayCells.map((cell, idx) => (
        <div key={idx} className="p-4 flex flex-col justify-between">
          <div className="text-[12px] text-text-2">{cell.label}</div>
          <div className="stat text-text mt-1">{cell.value}</div>
          {cell.detail && (
            <div className="text-[12px] text-text-3 mt-1">{cell.detail}</div>
          )}
        </div>
      ))}
    </div>
  );
};

