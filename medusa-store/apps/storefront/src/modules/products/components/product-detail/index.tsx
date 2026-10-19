import { ArrowUpRightMini } from "@medusajs/icons"
import { HttpTypes } from "@medusajs/types"
import { getProductCopy } from "../../i18n"
import styles from "./styles.module.css"

export default function ProductDetails({ product, locale }: { product: HttpTypes.StoreProduct; locale: string }) {
  const copy = getProductCopy(locale)
  const raw = product.metadata?.apple_model_specs
  const specs = raw && typeof raw === "object" && !Array.isArray(raw) ? raw as Record<string, unknown> : product.metadata ?? {}
  const value = (key: string) => typeof specs[key] === "string" || typeof specs[key] === "number" ? String(specs[key]) : ""
  const display = [value("display_size_inches") && value("display_size_inches") + "″", value("display_name")].filter(Boolean).join(" · ")
  const dimensions = [value("height_mm"), value("width_mm"), value("depth_mm")]
  const rows = [
    [copy.release, value("release_year")], [copy.display, display],
    [copy.resolution, value("display_resolution_px")], [copy.chip, value("chip")],
    [copy.camera, value("rear_camera_system") || value("rear_camera_count")],
    [copy.frontCamera, value("front_camera")],
    [copy.dimensions, dimensions.every(Boolean) ? dimensions.join(" × ") + " mm" : ""],
    [copy.weight, value("weight_g") && value("weight_g") + " g"],
    [copy.connector, value("connector")], [copy.authentication, value("biometric")],
    [copy.modelNumbers, Array.isArray(specs.apple_model_numbers) ? specs.apple_model_numbers.join(", ") : ""],
  ].filter((row) => row[1])
  const highlights = [[copy.display, value("display_size_inches") && value("display_size_inches") + "″"], [copy.chip, value("chip")], [copy.camera, value("rear_camera_system") || value("rear_camera_count")]].filter((row) => row[1])
  const source = value("tech_specs_url")
  const safeSource = /^https:\/\/(support\.)?apple\.com\//.test(source)
  return <section className={styles.details} aria-labelledby="product-details-title">
    <h2 id="product-details-title" className={styles.sectionTitle}>{copy.details}</h2>
    {!!highlights.length && <div className={styles.highlights}>{highlights.map(([label, detail]) => <div className={styles.highlight} key={label}><span>{label}</span><strong>{detail}</strong></div>)}</div>}
    {product.description && <details className={styles.specs}><summary>{copy.description}</summary><p className={styles.description + " pb-6"} data-testid="product-description">{product.description}</p></details>}
    {!!rows.length && <details className={styles.specs} open><summary>{copy.specs}</summary><p className={styles.muted + " mb-6"}>{copy.specNote}</p><dl>{rows.map(([label, detail]) => <div className={styles.specRow} key={label}><dt>{label}</dt><dd>{detail}</dd></div>)}</dl>{safeSource && <a className={styles.source} href={source} target="_blank" rel="noopener noreferrer">{copy.appleSpecs} <ArrowUpRightMini aria-hidden="true" width={16} height={16} className="inline-block align-middle shrink-0" /></a>}</details>}
  </section>
}
