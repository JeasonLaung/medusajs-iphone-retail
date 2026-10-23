"use client"

import { Heading, Input, SubmitButton } from "@modules/account/components/ui"

import { useAccountCopy } from "@modules/account/use-account-copy"
import { createTransferRequest } from "@lib/data/orders"
import { Alert } from "antd"
import { useActionState } from "react"
// TODO: Re-add Toaster component when needed
import { useEffect, useState } from "react"

export default function TransferRequestForm() {
  const { t } = useAccountCopy()
  const [showSuccess, setShowSuccess] = useState(false)

  const [state, formAction] = useActionState(createTransferRequest, {
    success: false,
    error: null,
    order: null,
  })

  useEffect(() => {
    if (state.success && state.order) {
      setShowSuccess(true)
    }
  }, [state.success, state.order])

  return (
    <div className="flex flex-col gap-y-4 w-full">
      <div className="grid sm:grid-cols-2 items-center gap-x-8 gap-y-4 w-full">
        <div className="flex flex-col gap-y-1">
          <Heading
            level="h3"
            className="!text-sm font-semibold text-neutral-950"
          >
            {t("transfers")}
          </Heading>
          <p className="text-small-regular text-neutral-500">
            {t("transferDescription")}
          </p>
        </div>
        <form
          action={formAction}
          className="flex flex-col gap-y-1 sm:items-end"
        >
          <div className="flex flex-col gap-y-2 w-full">
            <Input
              className="w-full"
              name="order_id"
              placeholder={t("orderId")}
            />
            <SubmitButton
              variant="secondary"
              size="small"
              className="w-fit whitespace-nowrap self-end"
            >
              {t("requestTransfer")}
            </SubmitButton>
          </div>
        </form>
      </div>
      {!state.success && state.error && (
        <Alert type="error" showIcon title={t("error")} />
      )}
      {showSuccess && (
        <Alert
          type="success"
          showIcon
          closable
          onClose={() => setShowSuccess(false)}
          title={t("transferRequested", { id: state.order?.id || "" })}
          description={t("transferEmail", { email: state.order?.email || "" })}
        />
      )}
    </div>
  )
}
