import type { DeviceCommandBody } from "../../api/admin/devices/validators"

export type OrderDeviceCommand = Extract<
  DeviceCommandBody,
  {
    action:
      | "assign"
      | "order_assign"
      | "supplier_assign"
      | "release"
      | "fulfill"
      | "ship"
      | "cancel_fulfillment"
      | "save_allocation"
  }
>

export type Device = {
  id: string
  serial_number: string | null
  imei: string | null
  imei2: string | null
  eid: string | null
  variant_id: string
  source_id: string
  state: string
  grade: string | null
  battery_health: number | null
  bin: string
}

export type Assignment = {
  id: string
  device_id: string
  item_id: string
  source_id: string
  status: string
  fulfillment_id: string | null
  tracking_number: string | null
  replacement_of: string | null
}

export type OrderDeviceData = {
  devices: Device[]
  assigned_devices: Device[]
  assignments: Assignment[]
  count: number
  sources: { id: string; name: string; type: "self" | "supplier" }[]
  managed_variants: { variant_id: string }[]
  order: {
    status: string
    fulfillments?: import("../../lib/order-device-capacity").AllocationFulfillment[]
    items: {
      id: string
      title: string
      variant_title: string
      variant_id: string
      quantity?: number | string
      requires_imei?: boolean
      detail?: {
        quantity?: unknown
        fulfilled_quantity?: unknown
        raw_quantity?: unknown
        raw_fulfilled_quantity?: unknown
      }
    }[]
  } | null
}

export type RunOrderDeviceCommand = (
  body: OrderDeviceCommand,
) => Promise<boolean>
