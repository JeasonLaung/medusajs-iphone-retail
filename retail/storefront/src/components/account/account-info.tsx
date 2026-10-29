"use client"

import { Button } from "./ui"

import { Alert, Collapse } from "antd"
import { useEffect, useState } from "react"
import { useFormStatus } from "react-dom"
import { useAccountCopy } from "./context"
import useToggleState from "./use-toggle-state"

type AccountInfoProps = {
  label: string
  currentInfo: string | React.ReactNode
  isSuccess?: boolean
  isError?: boolean
  errorMessage?: string
  clearState: () => void
  children?: React.ReactNode
  "data-testid"?: string
}

export default function AccountInfo({
  label,
  currentInfo,
  isSuccess,
  isError,
  clearState,
  children,
  "data-testid": testId,
}: AccountInfoProps) {
  const { t } = useAccountCopy()
  const { state, close, toggle } = useToggleState()
  const { pending } = useFormStatus()
  const [editVersion, setEditVersion] = useState(0)
  useEffect(() => {
    if (isSuccess) close()
  }, [isSuccess, close])
  return (
    <div data-testid={testId}>
      <Collapse
        ghost
        activeKey={state ? ["edit"] : []}
        onChange={() => {
          if (state) setEditVersion((version) => version + 1)
          clearState()
          toggle()
        }}
        items={[
          {
            key: "edit",
            label: (
              <div className="flex flex-col gap-1">
                <span>{label}</span>
                <div className="font-semibold" data-testid="current-info">
                  {currentInfo}
                </div>
              </div>
            ),
            extra: (
              <Button
                variant="secondary"
                disabled={pending}
                onClick={(event) => {
                  event.stopPropagation()
                  if (state) setEditVersion((version) => version + 1)
                  clearState()
                  toggle()
                }}
                data-testid="edit-button"
                data-active={state}
              >
                {state ? t("cancel") : t("edit")}
              </Button>
            ),
            children: (
              <div key={editVersion} className="flex flex-col gap-4">
                {children}
                <Button
                  type="submit"
                  isLoading={pending}
                  className="self-end"
                  data-testid="save-button"
                >
                  {t("saveChanges")}
                </Button>
              </div>
            ),
          },
        ]}
      />
      {isSuccess && (
        <Alert
          type="success"
          showIcon
          message={t("updated", { label })}
          data-testid="success-message"
        />
      )}
      {isError && (
        <Alert
          type="error"
          showIcon
          message={t("error")}
          data-testid="error-message"
        />
      )}
    </div>
  )
}
