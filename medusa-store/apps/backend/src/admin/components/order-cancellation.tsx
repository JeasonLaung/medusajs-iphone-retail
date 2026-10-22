import { useEffect, useId, useState } from "react"
import { Button, Label, Prompt, Text, Textarea, toast } from "@medusajs/ui"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { sdk } from "../lib/sdk"
import en from "../i18n/json/order-cancellation-en.json"
import zh from "../i18n/json/order-cancellation-zh.json"

const translationOptions = { bindI18n: "languageChanged", bindI18nStore: "added" }

function useCancellationCopy() {
  const { t, i18n } = useTranslation(undefined, translationOptions)
  useEffect(() => {
    i18n.addResourceBundle("en", "translation", { orderCancellation: en }, true, true)
    i18n.addResourceBundle("zhCN", "translation", { orderCancellation: zh }, true, true)
  }, [i18n])
  return t
}

export function useOrderCancellation(order: { id: string; display_id: number }) {
  const t = useCancellationCopy()
  const client = useQueryClient()
  const fieldId = useId()
  const [open, setOpen] = useState(false)
  const [reason, setReason] = useState("")
  const [invalid, setInvalid] = useState(false)
  const mutation = useMutation({
    mutationFn: (reason: string) => sdk.client.fetch(`/admin/orders/${order.id}/cancel`, { method: "POST", body: { reason } }),
    onSuccess: async () => {
      setOpen(false)
      setReason("")
      toast.success(t("orders.orderCanceled"))
      await Promise.all([
        client.invalidateQueries({ queryKey: ["orders"] }),
        client.invalidateQueries({ queryKey: [["orders", "detail", order.id], "preview"] }),
      ])
    },
    onError: (error: Error) => toast.error(error.message),
  })
  const submit = () => {
    const trimmed = reason.trim()
    if (!trimmed || trimmed.length > 1000) { setInvalid(true); return }
    if (!mutation.isPending) mutation.mutate(trimmed)
  }
  return {
    open: () => { setReason(""); setInvalid(false); setOpen(true) },
    dialog: <Prompt variant="danger" open={open} onOpenChange={value => { if (!mutation.isPending) setOpen(value) }}>
      <Prompt.Content>
        <Prompt.Header>
          <Prompt.Title>{t("general.areYouSure")}</Prompt.Title>
          <Prompt.Description>{t("orders.cancelWarning", { id: `#${order.display_id}` })}</Prompt.Description>
        </Prompt.Header>
        <div className="flex flex-col gap-2 px-6 pt-4">
          <Label htmlFor={fieldId}>{t("orderCancellation.reason")} *</Label>
          <Textarea id={fieldId} required maxLength={1000} rows={4} value={reason} disabled={mutation.isPending}
            placeholder={t("orderCancellation.placeholder")} aria-invalid={invalid}
            aria-describedby={invalid ? `${fieldId}-error` : undefined}
            onChange={event => { setReason(event.target.value); setInvalid(false) }} />
          {invalid && <Text id={`${fieldId}-error`} role="alert" size="small" className="text-ui-fg-error">{t("orderCancellation.required")}</Text>}
        </div>
        <Prompt.Footer>
          <Prompt.Cancel disabled={mutation.isPending}>{t("orderCancellation.back")}</Prompt.Cancel>
          <Button size="small" variant="danger" isLoading={mutation.isPending} disabled={mutation.isPending} onClick={submit}>{t("orderCancellation.confirm")}</Button>
        </Prompt.Footer>
      </Prompt.Content>
    </Prompt>,
  }
}

export function OrderCancellationReason({ cancellation }: { cancellation: unknown }) {
  const t = useCancellationCopy()
  const reason = cancellation && typeof cancellation === "object" && "reason" in cancellation && typeof cancellation.reason === "string" ? cancellation.reason : null
  return <Text size="small" className="mt-2 whitespace-pre-wrap break-words text-ui-fg-subtle">
    {reason ? `${t("orderCancellation.reason")}: ${reason}` : t("orderCancellation.missing")}
  </Text>
}
