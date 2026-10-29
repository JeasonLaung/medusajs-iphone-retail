"use client"

import { Input, CountrySelect } from "./ui"

import { useAccountCopy } from "./context"

import React, { useActionState, useEffect, useMemo } from "react"

import { getCountryOptions } from "../../lib/countries"

import {useAddressActions} from "./context"
import type {Region} from "../../lib/regions"
import type {Customer,Address} from "../../lib/customer-client"
import AccountInfo from "./account-info"

type MyInformationProps = {
  customer: Customer & {addresses:Address[]}
  regions: Region[]
}

const ProfileBillingAddress: React.FC<MyInformationProps> = ({
  customer,
  regions,
}) => {
  const { t, locale } = useAccountCopy()
  const {addCustomerAddress,updateCustomerAddress}=useAddressActions(true)
  const regionOptions = useMemo(
    () => getCountryOptions(regions, locale),
    [regions, locale]
  )

  const [successState, setSuccessState] = React.useState(false)

  const billingAddress = customer.addresses?.find(
    (addr) => addr.is_default_billing
  )

  const initialState: Record<string, unknown> = {
    isDefaultBilling: true,
    isDefaultShipping: false,
    error: false,
    success: false,
  }

  if (billingAddress) {
    initialState.addressId = billingAddress.id
  }

  const [state, formAction] = useActionState(
    billingAddress ? updateCustomerAddress : addCustomerAddress,
    initialState
  )

  const clearState = () => {
    setSuccessState(false)
  }

  useEffect(() => {
    setSuccessState(!!state.success)
  }, [state])

  const currentInfo = useMemo(() => {
    if (!billingAddress) {
      return t("noBillingAddress")
    }

    const country =
      regionOptions?.find(
        (country) => country?.value === billingAddress.country_code
      )?.label || billingAddress.country_code?.toUpperCase()

    return (
      <div className="flex flex-col font-semibold" data-testid="current-info">
        <span>
          {billingAddress.first_name} {billingAddress.last_name}
        </span>
        <span>{billingAddress.company}</span>
        <span>
          {billingAddress.address_1}
          {billingAddress.address_2 ? `, ${billingAddress.address_2}` : ""}
        </span>
        <span>
          {billingAddress.postal_code}, {billingAddress.city}
        </span>
        <span>{country}</span>
      </div>
    )
  }, [billingAddress, regionOptions, t])

  return (
    <form action={formAction} onReset={() => clearState()} className="w-full">
      <input type="hidden" name="addressId" value={billingAddress?.id} />
      <AccountInfo
        label={t("billingAddress")}
        currentInfo={currentInfo}
        isSuccess={successState}
        isError={!!state.error}
        clearState={clearState}
        data-testid="account-billing-address-editor"
      >
        <div className="grid grid-cols-1 gap-y-2">
          <div className="grid grid-cols-2 gap-x-2">
            <Input
              label={t("firstName")}
              name="first_name"
              defaultValue={billingAddress?.first_name || undefined}
              required
              data-testid="billing-first-name-input"
            />
            <Input
              label={t("lastName")}
              name="last_name"
              defaultValue={billingAddress?.last_name || undefined}
              required
              data-testid="billing-last-name-input"
            />
          </div>
          <Input
            label={t("company")}
            name="company"
            defaultValue={billingAddress?.company || undefined}
            data-testid="billing-company-input"
          />
          <Input
            label={t("phone")}
            name="phone"
            type="phone"
            autoComplete="phone"
            required
            defaultValue={billingAddress?.phone ?? customer?.phone ?? ""}
            data-testid="billing-phone-input"
          />
          <Input
            label={t("address")}
            name="address_1"
            defaultValue={billingAddress?.address_1 || undefined}
            required
            data-testid="billing-address-1-input"
          />
          <Input
            label={t("address2")}
            name="address_2"
            defaultValue={billingAddress?.address_2 || undefined}
            data-testid="billing-address-2-input"
          />
          <div className="grid grid-cols-[144px_1fr] gap-x-2">
            <Input
              label={t("postalCode")}
              name="postal_code"
              defaultValue={billingAddress?.postal_code || undefined}
              required
              data-testid="billing-postcal-code-input"
            />
            <Input
              label={t("city")}
              name="city"
              defaultValue={billingAddress?.city || undefined}
              required
              data-testid="billing-city-input"
            />
          </div>
          <Input
            label={t("province")}
            name="province"
            defaultValue={billingAddress?.province || undefined}
            data-testid="billing-province-input"
          />
          <CountrySelect
            regions={regions}
            name="country_code"
            defaultValue={billingAddress?.country_code || undefined}
            autoComplete="country"
            required
            data-testid="billing-country-code-select"
          />
        </div>
      </AccountInfo>
    </form>
  )
}

export default ProfileBillingAddress
