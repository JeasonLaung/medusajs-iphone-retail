import type { Region } from "./regions"
import europeanCountries from "../data/european-countries.json"
import countryNameOverrides from "../data/country-name-overrides.json"

// Europe includes all UN M49 European countries/areas plus AM, AZ, CY, GE,
// KZ and TR for wider European market coverage. XK is a user-assigned code
// commonly used for Kosovo, not an officially assigned ISO 3166-1 code.
// Source: https://unstats.un.org/unsd/methodology/m49/overview/
const englishNames = new Intl.DisplayNames(["en"], {
  type: "region",
  fallback: "none",
})
const chineseNames = new Intl.DisplayNames(["zh"], {
  type: "region",
  fallback: "none",
})

export const normalizeCountrySearch = (value: string) =>
  value
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .trim()
    .toLowerCase()

export const getCountryOptions = (
  regions: Region[],
  locale = "en"
) => {
  // Keep renamed countries consistent across Node and browser ICU versions.
  const overrides: Record<string, string> =
    countryNameOverrides[locale.toLowerCase().startsWith("zh") ? "zh" : "en"]
  const names = locale.toLowerCase().startsWith("zh")
    ? chineseNames
    : englishNames
  const countries = new Map<
    string,
    { value: string; code: string; label: string; search: string }
  >()

  for (const country of europeanCountries) {
    const { code, alpha3, numeric } = country
    const label = overrides[code] ?? names.of(code)
    if (!label) continue
    countries.set(code, {
      value: code.toLowerCase(),
      code,
      label,
      search: normalizeCountrySearch(
        [
          code,
          alpha3,
          numeric,
          numeric ? String(Number(numeric)) : "",
          englishNames.of(code),
          chineseNames.of(code),
          code === "GB" ? "UK Britain Great Britain 英国" : "",
          code === "VA" ? "Holy See Vatican 梵蒂冈" : "",
        ]
          .filter(Boolean)
          .join(" ")
      ),
    })
  }

  for (const region of regions) {
    for (const country of region.countries ?? []) {
      const code = country.iso_2?.toUpperCase()
      if (!code || !/^[A-Z]{2}$/.test(code)) continue
      const label = overrides[code] ?? names.of(code)
      if (!label) continue
      countries.set(code, {
        value: code.toLowerCase(),
        code,
        label,
        search: normalizeCountrySearch(
          [
            countries.get(code)?.search,
            code,
            country.iso_3,
            country.num_code,
            label,
            englishNames.of(code),
            chineseNames.of(code),
            country.display_name,
            country.name,
          ]
            .filter(Boolean)
            .join(" ")
        ),
      })
    }
  }

  return Array.from(countries.values()).sort((a, b) =>
    a.label.localeCompare(
      b.label,
      locale.toLowerCase().startsWith("zh") ? "zh" : "en"
    )
  )
}
