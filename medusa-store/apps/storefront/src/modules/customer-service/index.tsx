"use client"

import { useEffect, useState } from "react"
import { isWithinServiceHours, whatsappLink, type ServiceContact } from "./contact"
import { getServiceCopy } from "./i18n"
import styles from "./styles.module.css"

type Props = { contacts: ServiceContact[]; locale: string | null; failed: boolean }
export default function CustomerService({ contacts, locale, failed }: Props) {
  const copy = getServiceCopy(locale)
  const [now, setNow] = useState<Date | null>(null)
  useEffect(() => {
    setNow(new Date())
    const interval = setInterval(() => setNow(new Date()), 60_000)
    return () => clearInterval(interval)
  }, [])
  return <section className={styles.contacts} aria-labelledby="service-title">
    <h2 id="service-title">{copy.title}</h2>
    <p className={styles.description}>{copy.description}</p>
    {failed || !contacts.length ? <p className={styles.empty}>{failed ? copy.unavailable : copy.empty}</p> : <>
      <div className={styles.groups}>{(["pre_sales", "after_sales"] as const).map(department => {
        const team = contacts.filter(contact => contact.department === department)
        if (!team.length) return null
        return <section key={department} aria-labelledby={`service-${department}`}>
          <h3 id={`service-${department}`}>{copy[department]}</h3>
          <p className={styles.description}>{department === "pre_sales" ? copy.preSalesDescription : copy.afterSalesDescription}</p>
          <ul className={styles.list}>{team.map(contact => {
            const whatsapp = whatsappLink(contact.whatsapp)
            const status = contact.availability === "schedule" ? now ? isWithinServiceHours(contact, now) ? copy.inHours : copy.outOfHours : copy.checking : copy[contact.availability]
            return <li key={contact.id} className={styles.person}>
              <div className={styles.heading}><h4>{contact.name}</h4><span className={styles.status}>{status}</span></div>
              <dl className={styles.hours}><dt>{copy.hours}</dt><dd>{[1, 2, 3, 4, 5, 6, 0].filter(day => contact.days.includes(day)).map(day => copy.days[day]).join(" / ")}<br /><time>{contact.start}</time>–<time>{contact.end}</time> ({contact.timezone})</dd></dl>
              <div className={styles.channels}>
                {whatsapp && <a href={whatsapp} target="_blank" rel="noopener noreferrer" aria-label={copy.whatsappLabel.replace("{name}", contact.name)}>{copy.whatsapp}: {contact.whatsapp}</a>}
                {contact.email && <a href={`mailto:${encodeURIComponent(contact.email)}`} aria-label={copy.emailLabel.replace("{name}", contact.name)}>{copy.email}: {contact.email}</a>}
              </div>
            </li>
          })}</ul>
        </section>
      })}</div>
      <p className={styles.note}>{copy.scheduleNote}</p>
    </>}
  </section>
}
