import React, { useRef } from 'react';
import {
  ColumnDef,
  RowData,
  flexRender,
  getCoreRowModel,
  useReactTable,
  SortingState,
  getSortedRowModel,
} from '@tanstack/react-table';
import { useVirtualizer } from '@tanstack/react-virtual';
import { ArrowUpIcon, ArrowDownIcon } from '@radix-ui/react-icons';
import { Button } from './Button';
import { Tooltip } from './Tooltip';

declare module '@tanstack/react-table' {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  interface ColumnMeta<TData extends RowData, TValue> {
    isNumeric?: boolean;
  }
}

export interface DataTableProps<TData> {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  columns: ColumnDef<TData, any>[];
  data: TData[];
  selectedId?: string | null;
  getRowId?: (row: TData) => string;
  onRowClick?: (row: TData) => void;
  // Offset pagination
  pageIndex?: number;
  pageSize?: number;
  totalCount?: number;
  onPageChange?: (page: number) => void;
  // Cursor pagination
  hasMore?: boolean;
  onLoadMore?: () => void;
  isLoadingMore?: boolean;
  className?: string;
}

export function DataTable<TData>({
  columns,
  data,
  selectedId,
  getRowId,
  onRowClick,
  pageIndex = 0,
  pageSize = 50,
  totalCount,
  onPageChange,
  hasMore,
  onLoadMore,
  isLoadingMore,
  className = '',
}: DataTableProps<TData>) {
  const [sorting, setSorting] = React.useState<SortingState>([]);
  const tableContainerRef = useRef<HTMLDivElement>(null);

  const table = useReactTable({
    data,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  const { rows } = table.getRowModel();
  const shouldVirtualize = rows.length > 100;

  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => tableContainerRef.current,
    estimateSize: () => 36,
    overscan: 10,
    enabled: shouldVirtualize,
  });

  const startIdx = pageIndex * pageSize + 1;
  const endIdx = Math.min((pageIndex + 1) * pageSize, totalCount ?? data.length);
  const total = totalCount ?? data.length;

  return (
    <div className={`w-full overflow-hidden flex flex-col ${className}`}>
      <div
        ref={tableContainerRef}
        className="w-full overflow-x-auto relative"
        style={{ maxHeight: shouldVirtualize ? '600px' : undefined }}
      >
        <table className="w-full border-collapse text-left">
          <thead className="sticky top-0 z-10 bg-bg-subtle border-b border-border">
            {table.getHeaderGroups().map((headerGroup) => (
              <tr key={headerGroup.id} className="h-8">
                {headerGroup.headers.map((header) => {
                  const isSorted = header.column.getIsSorted();
                  const isNumeric = header.column.columnDef.meta?.isNumeric;
                  return (
                    <th
                      key={header.id}
                      onClick={header.column.getToggleSortingHandler()}
                      className={`px-3 text-[12px] font-medium text-text-2 select-none ${
                        isNumeric ? 'text-right' : 'text-left'
                      } ${header.column.getCanSort() ? 'cursor-pointer hover:text-text' : ''}`}
                    >
                      <div
                        className={`inline-flex items-center gap-1 ${
                          isNumeric ? 'justify-end w-full' : ''
                        }`}
                      >
                        {flexRender(header.column.columnDef.header, header.getContext())}
                        {isSorted === 'asc' && (
                          <ArrowUpIcon className="w-3 h-3 text-text shrink-0" />
                        )}
                        {isSorted === 'desc' && (
                          <ArrowDownIcon className="w-3 h-3 text-text shrink-0" />
                        )}
                      </div>
                    </th>
                  );
                })}
              </tr>
            ))}
          </thead>
          <tbody>
            {shouldVirtualize ? (
              <>
                <tr style={{ height: `${rowVirtualizer.getVirtualItems()[0]?.start ?? 0}px` }} />
                {rowVirtualizer.getVirtualItems().map((virtualRow) => {
                  const row = rows[virtualRow.index];
                  const rowId = getRowId ? getRowId(row.original) : String(virtualRow.index);
                  const isSelected = selectedId === rowId;
                  return (
                    <tr
                      key={row.id}
                      onClick={() => onRowClick && onRowClick(row.original)}
                      className={`h-9 border-b border-border text-[13px] text-text transition-colors ${
                        isSelected ? 'bg-bg-selected' : 'hover:bg-bg-muted'
                      } ${onRowClick ? 'cursor-pointer' : ''}`}
                    >
                      {row.getVisibleCells().map((cell) => {
                        const isNumeric = cell.column.columnDef.meta?.isNumeric;
                        return (
                          <td
                            key={cell.id}
                            className={`px-3 ${
                              isNumeric ? 'text-right tabular-nums' : 'text-left'
                            }`}
                          >
                            {flexRender(cell.column.columnDef.cell, cell.getContext())}
                          </td>
                        );
                      })}
                    </tr>
                  );
                })}
                <tr
                  style={{
                    height: `${
                      rowVirtualizer.getTotalSize() -
                      (rowVirtualizer.getVirtualItems()[rowVirtualizer.getVirtualItems().length - 1]
                        ?.end ?? 0)
                    }}px`,
                  }}
                />
              </>
            ) : (
              rows.map((row) => {
                const rowId = getRowId ? getRowId(row.original) : row.id;
                const isSelected = selectedId === rowId;
                return (
                  <tr
                    key={row.id}
                    onClick={() => onRowClick && onRowClick(row.original)}
                    className={`h-9 border-b border-border text-[13px] text-text transition-colors ${
                      isSelected ? 'bg-bg-selected' : 'hover:bg-bg-muted'
                    } ${onRowClick ? 'cursor-pointer' : ''}`}
                  >
                    {row.getVisibleCells().map((cell) => {
                      const isNumeric = cell.column.columnDef.meta?.isNumeric;
                      return (
                        <td
                          key={cell.id}
                          className={`px-3 ${
                            isNumeric ? 'text-right tabular-nums' : 'text-left'
                          }`}
                        >
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </td>
                      );
                    })}
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination Footer */}
      {onPageChange && total > 0 && (
        <div className="h-10 px-4 border-t border-border flex items-center justify-between text-[12px] text-text-2 bg-bg">
          <div>
            {startIdx}-{endIdx} of {total.toLocaleString('en-US')}
          </div>
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="secondary"
              disabled={pageIndex <= 0}
              onClick={() => onPageChange(pageIndex - 1)}
            >
              Previous
            </Button>
            <Button
              size="sm"
              variant="secondary"
              disabled={endIdx >= total}
              onClick={() => onPageChange(pageIndex + 1)}
            >
              Next
            </Button>
          </div>
        </div>
      )}

      {/* Cursor Pagination */}
      {onLoadMore && hasMore && (
        <div className="h-10 px-4 border-t border-border flex items-center justify-center bg-bg">
          <Button size="sm" variant="ghost" loading={isLoadingMore} onClick={onLoadMore}>
            Load more
          </Button>
        </div>
      )}
    </div>
  );
}

/** Helper cell component to truncate text and show full text in tooltip */
export const TruncatedCell: React.FC<{ text?: string | null; maxWidth?: string; mono?: boolean }> = ({
  text,
  maxWidth = 'max-w-xs',
  mono = false,
}) => {
  if (!text) return <span>—</span>;
  return (
    <Tooltip content={text}>
      <span className={`block truncate ${maxWidth} ${mono ? 'mono-ui' : ''}`}>
        {text}
      </span>
    </Tooltip>
  );
};

