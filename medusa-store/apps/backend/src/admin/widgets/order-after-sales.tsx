import { defineWidgetConfig } from "@medusajs/admin-sdk"
import { AdminOrder, DetailWidgetProps } from "@medusajs/framework/types"
import { Button, Container, Heading, Text } from "@medusajs/ui"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

type Request = {
  id: string
  type: "return" | "exchange" | "repair"
  status: "pending" | "processing" | "completed" | "rejected"
  reason: string
  items: { item_id: string; title: string; quantity: number }[]
  created_at: string
}
const types = { return: "Return & refund", exchange: "Exchange", repair: "Repair / warranty" }
const statuses = { pending: "Awaiting review", processing: "In progress", completed: "Completed", rejected: "Declined" }

const OrderAfterSales = ({ data: order }: DetailWidgetProps<AdminOrder>) => {
  const client = useQueryClient()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const key = ["order-after-sales", order.id]
  const { data, isLoading, isError } = useQuery<{ requests: Request[] }>({
    queryKey: key,
    queryFn: async () => {
      const response = await fetch(`/admin/orders/${order.id}/after-sales`, { credentials: "include" })
      if (!response.ok) throw new Error("Unable to load requests")
      return response.json()
    },
  })

  async function update(request_id: string, status: "processing" | "completed" | "rejected") {
    setBusy(true)
    setError(null)
    try {
      const response = await fetch(`/admin/orders/${order.id}/after-sales`, {
        method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ request_id, status }),
      })
      if (!response.ok) throw new Error("Unable to update request. Refresh and try again.")
      await client.invalidateQueries({ queryKey: key })
    } catch (error) { setError((error as Error).message) }
    finally { setBusy(false) }
  }

  return <Container className="divide-y p-0">
    <div className="px-6 py-4"><Heading level="h2">After-sales requests</Heading><Text size="small" className="text-ui-fg-subtle">Review the customer's request, then arrange returns, refunds or replacements through the order actions.</Text></div>
    <div className="space-y-4 px-6 py-4">
      {isLoading && <Text>Loading requests…</Text>}
      {isError && <Text role="alert">Unable to load requests.</Text>}
      {error && <Text role="alert" className="text-ui-fg-error">{error}</Text>}
      {data?.requests.length === 0 && <Text>No after-sales requests.</Text>}
      {data?.requests.map((request) => <div key={request.id} className="space-y-2 rounded-lg border p-4">
        <div className="flex justify-between gap-3"><Text weight="plus">{types[request.type]}</Text><Text>{statuses[request.status]}</Text></div>
        <Text size="small">{request.id} · {new Date(request.created_at).toLocaleString()}</Text>
        <Text className="whitespace-pre-wrap break-words">{request.reason}</Text>
        {request.items.map((item) => <Text key={item.item_id} size="small">{item.title} × {item.quantity}</Text>)}
        {["pending", "processing"].includes(request.status) && <div className="flex flex-wrap gap-2 pt-2">
          {request.status === "pending" && <Button size="small" variant="secondary" disabled={busy} onClick={() => update(request.id, "processing")}>Start processing</Button>}
          <Button size="small" variant="secondary" disabled={busy} onClick={() => update(request.id, "completed")}>Mark completed</Button>
          <Button size="small" variant="secondary" disabled={busy} onClick={() => update(request.id, "rejected")}>Decline</Button>
        </div>}
      </div>)}
    </div>
  </Container>
}

export const config = defineWidgetConfig({ zone: "order.details.after" })
export default OrderAfterSales
