import { Metadata } from "next"
import Image from "next/image"
import { getLocale } from "@lib/data/locale-actions"
import { ArrowRight } from "@medusajs/icons"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { getSellCopy } from "@modules/sell/i18n"
import styles from "@modules/sell/styles.module.css"

export async function generateMetadata(): Promise<Metadata> {
  const copy = getSellCopy(await getLocale())
  return { title: copy.title, description: copy.intro }
}

export default async function Page() {
  const copy = getSellCopy(await getLocale())
  return <main className={styles.page}>
    <div className={styles.container}>
      <div className={styles.hero}>
        <div className={styles.heroCopy}>
          <h1>{copy.title}</h1>
          <p className={styles.description}>{copy.intro}</p>
          <LocalizedClientLink href="/sell/iphone/model" className={styles.primary}>
            {copy.start} <ArrowRight aria-hidden="true" />
          </LocalizedClientLink>
          <p className={styles.note}>{copy.availability}</p>
        </div>
        <div className={styles.heroImage}>
          <Image src="/images/sell/iphones-in-hand.png" alt={copy.imageAlt} width={452} height={532}
            sizes="(max-width: 699px) calc(100vw - 40px), 375px" priority />
        </div>
      </div>
      <section className={styles.process} aria-labelledby="sell-process-title">
        <h2 id="sell-process-title">{copy.processTitle}</h2>
        <p className={styles.processIntro}>{copy.processIntro}</p>
        <ol className={styles.processSteps}>
          {copy.processSteps.map((step, index) => <li key={index}>
            <p className={styles.stepLabel}>{copy.stepLabel.replace("{number}", String(index + 1))}</p>
            <h3>{step.title}</h3>
            <p className={styles.stepDescription}>{step.description}</p>
          </li>)}
        </ol>
      </section>
      <section className={styles.help}><h2>{copy.onePhone}</h2><p>{copy.onePhoneDescription}</p></section>
      <section className={styles.help}><h2>{copy.findModel}</h2><p>{copy.findModelDescription}</p></section>
    </div>
  </main>
}
