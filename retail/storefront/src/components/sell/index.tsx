"use client"

import { Button, Collapse, Steps } from "antd"
import { ArrowRight } from "../products/icons"
import { getSellCopy } from "./i18n"
import { getBuybackCopy } from "./buyback-copy"
import SellTheme from "../products/storefront-theme"
import styles from "./styles.module.css"

export default function SellLanding({ locale, segment: countryCode }: { locale: string | null; segment: string }) {
  const copy = getSellCopy(locale)
  const buyback = getBuybackCopy(locale)
  return <SellTheme locale={locale}><main className={styles.page}>
    <div className={styles.container}>
      <section className={styles.hero}>
        <div className={styles.heroCopy}>
          <h1>{copy.title}</h1>
          <p className={styles.description}>{copy.intro}</p>
          <div className={styles.actions}>
            <Button type="primary" size="large" href={`/${countryCode}/sell/iphone/model`} icon={<ArrowRight aria-hidden="true" />} iconPosition="end">{copy.start}</Button>
            <Button size="large" href={`/${countryCode}/account/buyback`}>{buyback.myRequests}</Button>
          </div>
        </div>
        <div className={styles.heroImage}><img src="/images/sell/iphones-in-hand.png" alt={copy.imageAlt} width={452} height={532} sizes="(max-width: 699px) 240px, 300px" loading="eager" decoding="async" style={{ color: "transparent" }} /></div>
      </section>
      <section className={styles.process} aria-labelledby="sell-process-title">
        <h2 id="sell-process-title">{copy.processTitle}</h2>
        <p className={styles.processIntro}>{copy.processIntro}</p>
        <Steps current={-1} responsive items={copy.processSteps.map(step => ({ title: step.title, description: step.description }))} />
      </section>
      <div className={styles.help}><Collapse bordered={false} defaultActiveKey={["model"]} items={[
        { key: "model", label: copy.findModel, children: <p>{copy.findModelDescription}</p> },
        { key: "phones", label: copy.onePhone, children: <p>{copy.onePhoneDescription}</p> },
      ]} /></div>
    </div>
  </main></SellTheme>
}
