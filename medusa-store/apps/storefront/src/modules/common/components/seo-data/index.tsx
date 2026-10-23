import { currentSEO, jsonLD, seoOrigin } from "@lib/data/seo"
import Script from "next/script"

export default async function SEOData() {
  const { settings: values, page } = await currentSEO(), origin = seoOrigin(values)
  const name = values["seo.site_name"] || "EuroRefurb"
  const structured = page?.structured_data && (Array.isArray(page.structured_data) ? page.structured_data.length > 0 : Object.keys(page.structured_data).length > 0)
    ? page.structured_data : [{ "@context": "https://schema.org", "@type": "Organization", name, url: origin }, { "@context": "https://schema.org", "@type": "WebSite", name, url: origin }]
  const measurement = values["ga4.measurement_id"]
  return <>
    <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: jsonLD(structured) }} />
    {values["analytics.enabled"] === "true" && /^G-[A-Z0-9]+$/.test(measurement || "") && <>
      <Script src={`https://www.googletagmanager.com/gtag/js?id=${measurement}`} strategy="afterInteractive" />
      <Script id="seo-ga4" strategy="afterInteractive">{`window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments)}gtag('js',new Date());gtag('config',${JSON.stringify(measurement)});`}</Script>
    </>}
  </>
}
