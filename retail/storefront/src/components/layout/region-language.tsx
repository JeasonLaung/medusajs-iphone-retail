// Original region/language DOM and interaction; caller owns the actual mutation.
"use client"

import {XMark} from "./icons"
import { Dialog, DialogPanel, DialogTitle } from "@headlessui/react"
import { useState, useTransition } from "react"

import ReactCountryFlag from "react-country-flag"
import type {Region} from "../../lib/regions"
import type {Locale,PreferenceTarget} from "../../lib/preferences"
import {preferenceTarget} from "../../lib/preferences"


import { getLayoutCopy } from "./i18n"
import { routeCountry } from "../../lib/storefront-route"

import styles from "./region-language.module.css"

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

export default function RegionLanguage({ regions, locales, locale,segment,onSelect,onRestore,canRestore }: {
  regions: Region[], locales: Locale[], locale: string | null,segment:string,
  onSelect:((target:PreferenceTarget)=>Promise<void>)|null,
  onRestore?:()=>Promise<void>,canRestore?:()=>boolean,
}) {
  const copy = getLayoutCopy(locale)
  const [open, setOpen] = useState(false)
  const [error, setError] = useState(false)
  const [recovery,setRecovery]=useState(false)
  const [retryChoice,setRetryChoice]=useState<{country:string;language:string}|null>(null)
  const [pending, startTransition] = useTransition()
  const routeSegment = segment
  const countryCode = routeCountry(routeSegment)

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
    setRetryChoice({country,language})
    startTransition(async () => {
      try {
        if(!onSelect)throw Error("Preferences unavailable")
        const target=preferenceTarget({regions,locales,segment:routeSegment,pathname:window.location.pathname,search:window.location.search,hash:window.location.hash,country,language})
        await onSelect(target)
        setOpen(false)
      } catch {
        setError(true)
        setRecovery(canRestore?.()??false)
        return
      }
    })
  }

  function restore(){
    setError(false)
    startTransition(async()=>{
      try{if(!onRestore)throw Error('Preferences unavailable');await onRestore();setRecovery(false);setOpen(false)}
      catch{setError(true);setRecovery(canRestore?.()??false)}
    })
  }

  return <>
    <button className={styles.trigger} onClick={() => { setError(false); setRecovery(canRestore?.()??false);setOpen(true) }} aria-label={copy.preferences} aria-haspopup="dialog" aria-expanded={open}>
      <CountryFlag country={countryCode} />
      <span>{languageName}</span>
    </button>
    <Dialog open={open} onClose={setOpen} className={styles.dialog}>
      <div className={styles.backdrop} aria-hidden="true" />
      <DialogPanel className={styles.drawer}>
        <div className={styles.heading}>
          <DialogTitle>{copy.selectRegion}</DialogTitle>
          <button onClick={() => setOpen(false)} aria-label={copy.closeRegion}><XMark aria-hidden="true" width={20} height={20} className="inline-block align-middle shrink-0" /></button>
        </div>
        {error && <p role="alert" className={styles.error}>{copy.error}</p>}
        {recovery&&<div className={styles.recovery} aria-live="polite" aria-busy={pending}>
          <p>{copy.pendingPreference}</p>
          <div>
            {retryChoice&&<button type="button" disabled={pending} onClick={()=>select(retryChoice.country,retryChoice.language)}>{copy.retryPreference}</button>}
            {onRestore&&<button type="button" disabled={pending} onClick={restore}>{copy.restorePreference}</button>}
          </div>
        </div>}
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
