"use client"

import { Dialog, DialogPanel, DialogTitle } from "@headlessui/react"
import { useState, useTransition } from "react"
import { useParams, usePathname } from "next/navigation"
import ReactCountryFlag from "react-country-flag"
import type { StoreRegion } from "@medusajs/types"
import type { Locale } from "@lib/data/locales"
import { updateLocale } from "@lib/data/locale-actions"
import { updateRegion } from "@lib/data/cart"
import { getLayoutCopy } from "@modules/layout/i18n"
import { routeCountry } from "@lib/util/storefront-route"
import { unstable_rethrow } from "next/navigation"
import styles from "./styles.module.css"

const countryLanguages: Record<string, string[]> = {
  dk: ["da"], be: ["nl", "fr"], de: ["de"], ee: ["et"], es: ["es"], fr: ["fr"],
  ie: [], it: ["it"], nl: ["nl"], pl: ["pl"], pt: ["pt"], sk: ["sk"], fi: ["fi", "sv"],
  se: ["sv"], at: ["de"], ch: ["de", "fr", "it"], cn: ["zh"], gb: [], us: [],
}
const countryNames: Record<string, string> = {
  dk: "Danmark", be: "België / Belgique", de: "Deutschland", ee: "Eesti", es: "España",
  fr: "France", ie: "Ireland", it: "Italia", nl: "Nederland", pl: "Polska", pt: "Portugal",
  sk: "Slovensko", fi: "Suomi", se: "Sverige", at: "Österreich", ch: "Schweiz / Suisse",
  cn: "中国", gb: "United Kingdom", us: "United States",
}
const baseLanguage = (code: string) => code.toLowerCase().split(/[-_]/)[0]
const localFlags = new Set(["dk", "fr", "de", "it", "es", "se", "gb"])
function CountryFlag({ country }: { country: string }) {
  return localFlags.has(country) ? (
    // Flag files are served locally so the selector works without a third-party image request.
    // eslint-disable-next-line @next/next/no-img-element
    <img src={`/flags/${country}.svg`} width={28} height={20} alt="" />
  ) : <ReactCountryFlag countryCode={country.toUpperCase()} svg style={{ width: 28, height: 20 }} aria-hidden="true" />
}

export default function RegionLanguage({ regions, locales, locale }: {
  regions: StoreRegion[], locales: Locale[], locale: string | null,
}) {
  const copy = getLayoutCopy(locale)
  const [open, setOpen] = useState(false)
  const [error, setError] = useState(false)
  const [pending, startTransition] = useTransition()
  const { countryCode: routeSegment } = useParams<{ countryCode: string }>()
  const countryCode = routeCountry(routeSegment)
  const pathname = usePathname()
  const available = [...locales]
  for (const fallback of [{ code: "en", name: "English" }, { code: "zh-CN", name: "简体中文" }]) {
    if (!available.some((entry) => baseLanguage(entry.code) === baseLanguage(fallback.code))) available.push(fallback)
  }
  const currentLanguage = baseLanguage(locale || "en")
  const languageName = available.find((entry) => baseLanguage(entry.code) === currentLanguage)?.name ?? "English"
  const countries = regions.flatMap((region) => region.countries ?? [])
    .filter((entry, index, all) => entry.iso_2 && all.findIndex((other) => other.iso_2 === entry.iso_2) === index)
    .sort((a, b) => {
      const order = Object.keys(countryNames)
      const rank = (code: string) => order.includes(code) ? order.indexOf(code) : order.length
      return rank(a.iso_2!) - rank(b.iso_2!) || (a.display_name ?? "").localeCompare(b.display_name ?? "")
    })

  function select(country: string, language: string) {
    setError(false)
    startTransition(async () => {
      try {
        await updateLocale(language)
        const rest = pathname.slice(routeSegment.length + 1) || "/"
        const query = window.location.search
        const hash = window.location.hash
        await updateRegion(country, `${rest}${query}${hash}`, language)
        setOpen(false)
      } catch (error) {
        unstable_rethrow(error)
        setError(true)
        return
      }
    })
  }

  return <>
    <button className={styles.trigger} onClick={() => { setError(false); setOpen(true) }} aria-label={copy.preferences} aria-haspopup="dialog" aria-expanded={open}>
      <CountryFlag country={countryCode} />
      <span>{languageName}</span>
    </button>
    <Dialog open={open} onClose={setOpen} className={styles.dialog}>
      <div className={styles.backdrop} aria-hidden="true" />
      <DialogPanel className={styles.drawer}>
        <div className={styles.heading}>
          <DialogTitle>{copy.selectRegion}</DialogTitle>
          <button onClick={() => setOpen(false)} aria-label={copy.closeRegion}>×</button>
        </div>
        {error && <p role="alert" className={styles.error}>{copy.error}</p>}
        <ul className={styles.list} aria-busy={pending}>
          {countries.map((entry) => {
            const country = entry.iso_2!
            const codes = [...(countryLanguages[country] ?? []), "en", "zh"]
            const languages = codes.flatMap((code) => {
              const language = available.find((option) => baseLanguage(option.code) === code)
              return language ? [language] : []
            })
            return <li className={styles.row} key={country}>
              <CountryFlag country={country} />
              <span className={styles.country}>{countryNames[country] ?? entry.display_name}</span>
              <div className={styles.languages}>
                {languages.map((language) => {
                  const selected = country === countryCode && baseLanguage(language.code) === currentLanguage
                  return <button key={language.code} disabled={pending} aria-pressed={selected}
                    aria-label={`${countryNames[country] ?? entry.display_name} — ${language.name}`}
                    onClick={() => select(country, language.code)}>
                    {baseLanguage(language.code) === "zh" ? "中文" : baseLanguage(language.code).toUpperCase()}
                  </button>
                })}
              </div>
            </li>
          })}
        </ul>
        {!countries.length && <p className={styles.error}>{copy.noRegions}</p>}
      </DialogPanel>
    </Dialog>
  </>
}
