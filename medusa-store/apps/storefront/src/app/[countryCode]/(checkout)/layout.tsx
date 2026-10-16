import LocalizedClientLink from "@modules/common/components/localized-client-link"
import ChevronDown from "@modules/common/icons/chevron-down"
import { getCheckoutCopy } from "@modules/checkout/i18n"
import styles from "@modules/checkout/styles.module.css"

export default async function CheckoutLayout({ children, params }: { children: React.ReactNode; params: Promise<{ countryCode: string }> }) {
  const { countryCode } = await params
  const copy = getCheckoutCopy(countryCode)
  return <div className={styles.layout}>
    <header className={styles.header}><nav>
      <LocalizedClientLink href="/cart" className={styles.back} data-testid="back-to-cart-link"><ChevronDown className="rotate-90" size={16} />{copy.back}</LocalizedClientLink>
      <LocalizedClientLink href="/" className={styles.brand} data-testid="store-link">Medusa Store</LocalizedClientLink>
    </nav></header>
    <div data-testid="checkout-container">{children}</div>
  </div>
}
