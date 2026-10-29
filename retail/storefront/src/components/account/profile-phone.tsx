"use client"

import { Input } from "./ui"

import { useAccountCopy } from "./context"

import React, { useEffect, useActionState } from "react"

import AccountInfo from "./account-info"
import type {Customer} from "../../lib/customer-client"
import {useAccountContext} from "./context"

type MyInformationProps = {
  customer: Customer
}

const ProfileEmail: React.FC<MyInformationProps> = ({ customer }) => {
  const { t } = useAccountCopy()
  const {api} = useAccountContext()
  const [successState, setSuccessState] = React.useState(false)

  const updateCustomerPhone = async (
    _currentState: Record<string, unknown>,
    formData: FormData
  ) => {
    const customer = {
      phone: formData.get("phone") as string,
    }

    try {
      await api.updateProfile(customer)
      return { success: true, error: null }
    } catch (error) {
      return { success: false, error: String(error) }
    }
  }

  const [state, formAction] = useActionState(updateCustomerPhone, {
    error: null as string | null,
    success: false,
  })

  const clearState = () => {
    setSuccessState(false)
  }

  useEffect(() => {
    setSuccessState(state.success)
  }, [state])

  return (
    <form action={formAction} className="w-full">
      <AccountInfo
        label={t("phone")}
        currentInfo={`${customer.phone}`}
        isSuccess={successState}
        isError={!!state.error}
        errorMessage={state.error || undefined}
        clearState={clearState}
        data-testid="account-phone-editor"
      >
        <div className="grid grid-cols-1 gap-y-2">
          <Input
            label={t("phone")}
            name="phone"
            type="phone"
            autoComplete="phone"
            required
            defaultValue={customer.phone ?? ""}
            data-testid="phone-input"
          />
        </div>
      </AccountInfo>
    </form>
  )
}

export default ProfileEmail
