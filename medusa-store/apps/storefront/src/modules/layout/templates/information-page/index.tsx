import { getLocale } from "@lib/data/locale-actions"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { getLayoutCopy } from "@modules/layout/i18n"
import styles from "./styles.module.css"

export type InformationPageKind = "sell" | "about" | "reviews" | "help"

export default async function InformationPage({ kind }: { kind: InformationPageKind }) {
  const copy = getLayoutCopy(await getLocale())
  return (
    <main className={styles.page}>
      <div className={styles.container}>
        <p className={styles.eyebrow}>{copy[kind]}</p>
        <h1>{copy[`${kind}Title`]}</h1>
        <p className={styles.description}>{copy[`${kind}Description`]}</p>
        {kind === "sell" && <span className={styles.badge}>{copy.upcoming}</span>}
        {kind === "about" && <section id="our-focus" className={styles.section}>
          <h2>{copy.focus}</h2>
          <p>{copy.focusDescription}</p>
        </section>}
        {kind === "help" && <div className={styles.sections}>
          <section id="shopping" className={styles.section}>
            <h2>{copy.shopping}</h2>
            <p>{copy.shoppingDescription}</p>
            <LocalizedClientLink href="/store">{copy.store} <span aria-hidden="true">→</span></LocalizedClientLink>
          </section>
          <section id="account" className={styles.section}>
            <h2>{copy.orders}</h2>
            <p>{copy.ordersDescription}</p>
            <LocalizedClientLink href="/account">{copy.account} <span aria-hidden="true">→</span></LocalizedClientLink>
          </section>
        </div>}
        <LocalizedClientLink href="/store" className={styles.back}>{copy.back} <span aria-hidden="true">→</span></LocalizedClientLink>
      </div>
    </main>
  )
}
