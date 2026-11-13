import React, { useState } from 'react';
import { Button } from './Button';
import { DataTable } from './DataTable';
import { ColumnDef } from '@tanstack/react-table';

export interface ChartContainerProps {
  title: string;
  timeRangeText?: string;
  // Tables of any row shape; TanStack's own column type is generic the same way.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  tableData?: any[];
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  tableColumns?: ColumnDef<any, any>[];
  children: React.ReactNode;
  className?: string;
}

export const ChartContainer: React.FC<ChartContainerProps> = ({
  title,
  timeRangeText,
  tableData,
  tableColumns,
  children,
  className = '',
}) => {
  const [showData, setShowData] = useState(false);

  return (
    <div className={`bg-bg border border-border rounded-md ${className}`}>
      {/* Header */}
      <div className="h-11 px-4 border-b border-border flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="section-title text-text">{title}</span>
          {timeRangeText && <span className="meta text-text-3">{timeRangeText}</span>}
        </div>
        {tableData && tableColumns && (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setShowData(!showData)}
          >
            {showData ? 'View chart' : 'View data'}
          </Button>
        )}
      </div>

      {/* Body: 240px */}
      <div className="p-4">
        {showData && tableData && tableColumns ? (
          <div className="h-[240px] overflow-auto">
            <DataTable columns={tableColumns} data={tableData} />
          </div>
        ) : (
          <div className="h-[240px] w-full">{children}</div>
        )}
      </div>
    </div>
  );
};

interface ChartPayloadEntry {
  name?: string;
  value?: number | string;
  color?: string;
  fill?: string;
}

interface ChartTooltipProps {
  active?: boolean;
  payload?: ChartPayloadEntry[];
  label?: string | number;
}

export const CustomChartTooltip = ({ active, payload, label }: ChartTooltipProps) => {
  if (!active || !payload || !payload.length) return null;

  return (
    <div className="bg-bg shadow-float rounded p-2.5 text-[12px] border border-border min-w-[160px]">
      <div className="text-text-2 mb-2 font-medium">{label}</div>
      <div className="space-y-1">
        {payload.map((entry, index) => (
          <div key={index} className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-1.5">
              <span
                className="w-2 h-2 shrink-0 inline-block"
                style={{ backgroundColor: entry.color || entry.fill }}
              />
              <span className="text-text truncate max-w-[120px]">{entry.name}</span>
            </div>
            <span className="text-text font-medium tabular-nums">
              {typeof entry.value === 'number' ? entry.value.toLocaleString('en-US') : entry.value}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
};

export const CustomChartLegend = ({ payload }: { payload?: ChartPayloadEntry[] }) => {
  if (!payload || !payload.length) return null;

  return (
    <div className="flex items-center gap-4 text-[12px] text-text-2 mb-3">
      {payload.map((entry, index) => (
        <div key={index} className="inline-flex items-center gap-1.5">
          <span
            className="w-2 h-2 shrink-0 inline-block"
            style={{ backgroundColor: entry.color }}
          />
          <span>{entry.value}</span>
        </div>
      ))}
    </div>
  );
};

