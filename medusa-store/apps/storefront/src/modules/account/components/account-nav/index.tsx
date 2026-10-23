"use client"

import { Menu, Typography } from "antd"
import { useParams, usePathname } from "next/navigation"
import { ArrowRightOnRectangle } from "@medusajs/icons"
import { signout } from "@lib/data/customer"
import type { HttpTypes } from "@medusajs/types"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { useAccountCopy } from "@modules/account/use-account-copy"
import { getBuybackCopy } from "@modules/sell/buyback-copy"
import { LinkButton } from "@modules/account/components/ui"
import ChevronDown from "@modules/common/icons/chevron-down"
import User from "@modules/common/icons/user"
import MapPin from "@modules/common/icons/map-pin"
import Package from "@modules/common/icons/package"

export default function AccountNav({
  customer,
}: {
  customer: HttpTypes.StoreCustomer | null
}) {
  const { t, locale } = useAccountCopy()
  const route = usePathname()
  const { countryCode } = useParams<{ countryCode: string }>()
  const selected = route?.replace("/" + countryCode, "") || "/account"
  const links = [
    {
      key: "/account",
      label: t("overview"),
      testId: "overview-link",
      icon: <User />,
    },
    {
      key: "/account/profile",
      label: t("profile"),
      testId: "profile-link",
      icon: <User />,
    },
    {
      key: "/account/addresses",
      label: t("addresses"),
      testId: "addresses-link",
      icon: <MapPin />,
    },
    {
      key: "/account/orders",
      label: t("orders"),
      testId: "orders-link",
      icon: <Package />,
    },
    {
      key: "/account/buyback",
      label: getBuybackCopy(locale).myRequests,
      testId: "buyback-link",
      icon: <Package />,
    },
  ]
  const menu = (
    <Menu
      mode="inline"
      style={{ borderInlineEnd: 0, background: "transparent" }}
      selectedKeys={[
        selected.startsWith("/account/orders/") ? "/account/orders"
          : selected.startsWith("/account/buyback/") ? "/account/buyback" : selected,
      ]}
      items={[
        ...links.map((link) => ({
          key: link.key,
          icon: link.icon,
          label: (
            <LocalizedClientLink href={link.key} data-testid={link.testId}>
              {link.label}
            </LocalizedClientLink>
          ),
        })),
        {
          key: "logout",
          icon: <ArrowRightOnRectangle />,
          label: <span data-testid="logout-button">{t("logout")}</span>,
          onClick: () => signout(countryCode),
        },
      ]}
    />
  )
  return (
    <nav aria-label={t("account")} className="mb-6 small:mb-0 small:pr-6">
      <div className="hidden small:block" data-testid="account-nav">
        <Typography.Title level={4}>{t("account")}</Typography.Title>
        {menu}
      </div>
      <div className="small:hidden" data-testid="mobile-account-nav">
        {selected === "/account" ? (
          <>
            <Typography.Title level={4}>
              {t("hello", { name: customer?.first_name || "" })}
            </Typography.Title>
            {menu}
          </>
        ) : (
          <LinkButton
            href="/account"
            variant="transparent"
            icon={<ChevronDown className="rotate-90" />}
            data-testid="account-main-link"
          >
            {t("account")}
          </LinkButton>
        )}
      </div>
    </nav>
  )
}
