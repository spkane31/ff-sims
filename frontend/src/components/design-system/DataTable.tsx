import type { ReactNode } from "react";
import { FOCUS_RING } from "@/components/design-system/focus-ring";

export interface DataTableColumn<T> {
  /** Unique key, also used as the sort field identifier passed to onSort. */
  id: string;
  header: string;
  /** Renders a cell's content for one row. */
  cell: (row: T) => ReactNode;
  /** Omit for non-sortable columns. */
  sortable?: boolean;
}

export interface DataTableProps<T> {
  columns: DataTableColumn<T>[];
  rows: T[];
  rowKey: (row: T) => string;
  sortField?: string;
  sortDirection?: "asc" | "desc";
  onSort?: (fieldId: string) => void;
}

export default function DataTable<T>({
  columns,
  rows,
  rowKey,
  sortField,
  sortDirection,
  onSort,
}: DataTableProps<T>) {
  return (
    <div className="overflow-x-auto">
      <table className="min-w-full border-collapse text-sm">
        <thead>
          <tr style={{ borderBottom: "1px solid var(--border-subtle)" }}>
            {columns.map((col) => (
              <th
                key={col.id}
                scope="col"
                className="whitespace-nowrap px-4 py-3 text-left text-xs font-medium uppercase tracking-wider"
                style={{ color: "var(--text-muted)" }}
              >
                {col.sortable ? (
                  <button
                    type="button"
                    onClick={() => onSort?.(col.id)}
                    className={`inline-flex items-center gap-1 ${FOCUS_RING}`}
                  >
                    {col.header}
                    {sortField === col.id && (
                      <span aria-hidden="true">
                        {sortDirection === "asc" ? "↑" : "↓"}
                      </span>
                    )}
                  </button>
                ) : (
                  col.header
                )}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr
              key={rowKey(row)}
              style={{
                backgroundColor:
                  i % 2 === 0 ? "var(--surface-raised)" : "var(--surface-sunken)",
                borderBottom: "1px solid var(--border-subtle)",
              }}
            >
              {columns.map((col) => (
                <td key={col.id} className="whitespace-nowrap px-4 py-4">
                  {col.cell(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
