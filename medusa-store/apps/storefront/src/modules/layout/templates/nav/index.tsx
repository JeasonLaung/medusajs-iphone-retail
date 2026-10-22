import { listLocales } from "@lib/data/locales"
import { getLocale } from "@lib/data/locale-actions"
import { listRegions } from "@lib/data/regions"
import { retrieveCart } from "@lib/data/cart"
import { retrieveCustomer } from "@lib/data/customer"
import StoreHeader from "@modules/layout/components/store-header"

export default async function Nav() {
  const [regions, locales, locale, cart, customer] = await Promise.all([
    listRegions(), listLocales(), getLocale(), retrieveCart().catch(() => null), retrieveCustomer(),
  ])

  return <StoreHeader regions={regions ?? []} locales={locales ?? []} locale={locale}
    isLoggedIn={!!customer}
    cartCount={cart?.items?.reduce((sum, item) => sum + item.quantity, 0) ?? 0} />
}
