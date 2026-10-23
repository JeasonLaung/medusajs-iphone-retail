import tr from "./translate"
import { Table as AntTable } from 'antd'
import type { TableProps } from 'antd'
import type { ColumnsType, ColumnType } from 'antd/es/table'
import { useEffect, useMemo, useRef, useState } from 'react'
import type { HTMLAttributes, KeyboardEvent, PointerEvent as ReactPointerEvent, ReactNode } from 'react'

const DEFAULT_COLUMN_WIDTH = 160
const MIN_COLUMN_WIDTH = 96
const KEYBOARD_RESIZE_STEP = 16

type RowData = object
type WidthMap = Record<string, number>

interface ResizeHeaderCellProps extends HTMLAttributes<HTMLTableCellElement> {
  children?: ReactNode
  columnKey?: string
  columnLabel?: string
  currentWidth?: number
  initialWidth?: number
  onColumnResize?: (key: string, width: number) => void
}

function numericWidth(width: ColumnType<RowData>['width']) {
  if (typeof width === 'number') return Math.max(MIN_COLUMN_WIDTH, width)
  if (typeof width === 'string' && /^\d+(\.\d+)?px$/.test(width)) {
    return Math.max(MIN_COLUMN_WIDTH, Number.parseFloat(width))
  }
  return DEFAULT_COLUMN_WIDTH
}

function columnIdentity<RecordType extends RowData>(column: ColumnsType<RecordType>[number], path: number[]) {
  if (column.key != null) return String(column.key)
  if ('dataIndex' in column && column.dataIndex != null) {
    return Array.isArray(column.dataIndex) ? column.dataIndex.join('.') : String(column.dataIndex)
  }
  return `column-${path.join('-')}`
}

function textLabel(value: unknown) {
  return typeof value === 'string' || typeof value === 'number' ? String(value) : tr("此列")
}

function ResizableHeaderCell({
  children,
  columnKey,
  columnLabel = tr("此列"),
  currentWidth,
  initialWidth,
  onColumnResize,
  ...cellProps
}: ResizeHeaderCellProps) {
  const cleanupRef = useRef<() => void>(() => undefined)

  useEffect(() => () => cleanupRef.current(), [])

  const resize = (nextWidth: number) => {
    if (!columnKey || !onColumnResize) return
    onColumnResize(columnKey, Math.max(MIN_COLUMN_WIDTH, Math.round(nextWidth)))
  }

  const startResize = (event: ReactPointerEvent<HTMLSpanElement>) => {
    if (!columnKey || !onColumnResize || currentWidth == null) return
    event.preventDefault()
    event.stopPropagation()

    const startX = event.clientX
    const startWidth = currentWidth
    document.body.classList.add('is-resizing-table-column')

    const onPointerMove = (moveEvent: PointerEvent) => resize(startWidth + moveEvent.clientX - startX)
    const stopResize = () => {
      document.removeEventListener('pointermove', onPointerMove)
      document.removeEventListener('pointerup', stopResize)
      document.removeEventListener('pointercancel', stopResize)
      document.body.classList.remove('is-resizing-table-column')
    }

    cleanupRef.current()
    cleanupRef.current = stopResize
    document.addEventListener('pointermove', onPointerMove)
    document.addEventListener('pointerup', stopResize)
    document.addEventListener('pointercancel', stopResize)
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLSpanElement>) => {
    if (currentWidth == null) return
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
    event.preventDefault()
    event.stopPropagation()
    resize(currentWidth + (event.key === 'ArrowRight' ? KEYBOARD_RESIZE_STEP : -KEYBOARD_RESIZE_STEP))
  }

  return (
    <th {...cellProps}>
      {children}
      {columnKey && currentWidth != null && (
        <span
          className="table-column-resize-handle"
          role="separator"
          aria-label={tr(`调整${columnLabel}宽度`)}
          aria-orientation="vertical"
          aria-valuemin={MIN_COLUMN_WIDTH}
          aria-valuenow={currentWidth}
          tabIndex={0}
          title={tr("拖拽调整列宽；方向键可微调")}
          onClick={event => event.stopPropagation()}
          onDoubleClick={event => {
            event.preventDefault()
            event.stopPropagation()
            if (initialWidth != null) resize(initialWidth)
          }}
          onKeyDown={handleKeyDown}
          onPointerDown={startResize}
        />
      )}
    </th>
  )
}

export function Table<RecordType extends RowData = RowData>({ columns, components, scroll, ...props }: TableProps<RecordType>) {
  const [widths, setWidths] = useState<WidthMap>({})

  const resizeColumn = (key: string, width: number) => {
    setWidths(current => current[key] === width ? current : { ...current, [key]: width })
  }

  const resizableColumns = useMemo(() => {
    const enhance = (items: ColumnsType<RecordType>, parentPath: number[] = []): ColumnsType<RecordType> => items.map((column, index) => {
      const path = [...parentPath, index]
      if ('children' in column && column.children) {
        return { ...column, children: enhance(column.children, path) }
      }

      const key = columnIdentity(column, path)
      const initialWidth = numericWidth(column.width)
      const width = widths[key] ?? initialWidth
      const existingHeaderCell = column.onHeaderCell

      return {
        ...column,
        width,
        onHeaderCell: headerColumn => ({
          ...(existingHeaderCell?.(headerColumn) ?? {}),
          columnKey: key,
          columnLabel: textLabel(column.title),
          currentWidth: width,
          initialWidth,
          onColumnResize: resizeColumn,
        }) as HTMLAttributes<HTMLElement>,
      }
    })

    return columns ? enhance(columns) : columns
  }, [columns, widths])

  const totalWidth = useMemo(() => {
    const sumLeaves = (items?: ColumnsType<RecordType>): number => items?.reduce((total, column) => {
      if ('children' in column && column.children) return total + sumLeaves(column.children)
      return total + numericWidth(column.width)
    }, 0) ?? 0
    return sumLeaves(resizableColumns)
  }, [resizableColumns])

  return (
    <AntTable<RecordType>
      {...props}
      columns={resizableColumns}
      components={{
        ...components,
        header: { ...components?.header, cell: ResizableHeaderCell },
      }}
      scroll={{ ...scroll, x: typeof scroll?.x === 'number' ? Math.max(scroll.x, totalWidth) : scroll?.x ?? totalWidth }}
    />
  )
}

export type { ColumnsType }
