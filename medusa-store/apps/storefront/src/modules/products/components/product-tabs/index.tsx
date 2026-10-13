"use client"

import Back from "@modules/common/icons/back"
import FastDelivery from "@modules/common/icons/fast-delivery"
import Refresh from "@modules/common/icons/refresh"

import Accordion from "./accordion"
import { HttpTypes } from "@medusajs/types"

type ProductTabsProps = {
  product: HttpTypes.StoreProduct
}

const ProductTabs = ({ product }: ProductTabsProps) => {
  const infoLabel = product.metadata?.apple_model_specs
    ? "Model specifications"
    : "Product Information"
  const tabs = [
    {
      label: infoLabel,
      component: <ProductInfoTab product={product} />,
    },
    {
      label: "Shipping & Returns",
      component: <ShippingInfoTab />,
    },
  ]

  return (
    <div className="w-full">
      <Accordion type="multiple" defaultValue={[infoLabel]}>
        {tabs.map((tab, i) => (
          <Accordion.Item
            key={i}
            title={tab.label}
            headingSize="medium"
            value={tab.label}
          >
            {tab.component}
          </Accordion.Item>
        ))}
      </Accordion>
    </div>
  )
}

const ProductInfoTab = ({ product }: ProductTabsProps) => {
  const rawSpecs = product.metadata?.apple_model_specs
  if (rawSpecs && typeof rawSpecs === "object" && !Array.isArray(rawSpecs)) {
    const specs = rawSpecs as Record<string, unknown>
    const value = (key: string) => {
      const item = specs[key]
      return typeof item === "string" || typeof item === "number"
        ? String(item)
        : null
    }
    const rows = [
      ["Release year", value("release_year")],
      [
        "Display",
        [
          value("display_size_inches") &&
            `${value("display_size_inches")} inch`,
          value("display_name"),
        ]
          .filter(Boolean)
          .join(" · "),
      ],
      [
        "Resolution",
        [
          value("display_resolution_px") &&
            `${value("display_resolution_px")} pixels`,
          value("pixel_density_ppi") && `${value("pixel_density_ppi")} ppi`,
        ]
          .filter(Boolean)
          .join(" · "),
      ],
      ["Chip", value("chip")],
      [
        "Rear cameras",
        [
          value("rear_camera_count") &&
            `${value("rear_camera_count")} ${
              value("rear_camera_count") === "1" ? "camera" : "cameras"
            }`,
          value("rear_camera_system"),
        ]
          .filter(Boolean)
          .join(" · "),
      ],
      ["Front camera", value("front_camera")],
      [
        "Size",
        [value("height_mm"), value("width_mm"), value("depth_mm")].every(
          Boolean
        )
          ? `${value("height_mm")} × ${value("width_mm")} × ${value(
              "depth_mm"
            )} mm (H × W × D)`
          : null,
      ],
      ["Weight", value("weight_g") && `${value("weight_g")} g`],
      ["Connector", value("connector")],
      ["Authentication", value("biometric")],
      [
        "Apple model numbers",
        Array.isArray(specs.apple_model_numbers)
          ? specs.apple_model_numbers.join(", ")
          : null,
      ],
    ].filter((row) => row[1])

    return (
      <div className="py-8 text-small-regular">
        <p className="mb-5 text-ui-fg-subtle">
          Apple model specifications. These describe the iPhone model, not this
          SKU&apos;s storage, condition, battery or stock.
        </p>
        <dl className="divide-y divide-ui-border-base border-y border-ui-border-base">
          {rows.map(([label, detail]) => (
            <div
              key={label}
              className="grid grid-cols-1 gap-1 py-3 sm:grid-cols-[160px_1fr] sm:gap-4"
            >
              <dt className="font-semibold">{label}</dt>
              <dd>{detail}</dd>
            </div>
          ))}
        </dl>
        {value("tech_specs_url") && (
          <a
            className="mt-5 inline-block text-ui-fg-interactive underline"
            href={value("tech_specs_url")!}
            target="_blank"
            rel="noopener noreferrer"
          >
            Apple technical specifications
          </a>
        )}
      </div>
    )
  }

  return (
    <div className="text-small-regular py-8">
      <div className="grid grid-cols-2 gap-x-8">
        <div className="flex flex-col gap-y-4">
          <div>
            <span className="font-semibold">Material</span>
            <p>{product.material ? product.material : "-"}</p>
          </div>
          <div>
            <span className="font-semibold">Country of origin</span>
            <p>{product.origin_country ? product.origin_country : "-"}</p>
          </div>
          <div>
            <span className="font-semibold">Type</span>
            <p>{product.type ? product.type.value : "-"}</p>
          </div>
        </div>
        <div className="flex flex-col gap-y-4">
          <div>
            <span className="font-semibold">Weight</span>
            <p>{product.weight ? `${product.weight} g` : "-"}</p>
          </div>
          <div>
            <span className="font-semibold">Dimensions</span>
            <p>
              {product.length && product.width && product.height
                ? `${product.length}L x ${product.width}W x ${product.height}H`
                : "-"}
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}

const ShippingInfoTab = () => {
  return (
    <div className="text-small-regular py-8">
      <div className="grid grid-cols-1 gap-y-8">
        <div className="flex items-start gap-x-2">
          <FastDelivery />
          <div>
            <span className="font-semibold">Fast delivery</span>
            <p className="max-w-sm">
              Your package will arrive in 3-5 business days at your pick up
              location or in the comfort of your home.
            </p>
          </div>
        </div>
        <div className="flex items-start gap-x-2">
          <Refresh />
          <div>
            <span className="font-semibold">Simple exchanges</span>
            <p className="max-w-sm">
              Is the fit not quite right? No worries - we&apos;ll exchange your
              product for a new one.
            </p>
          </div>
        </div>
        <div className="flex items-start gap-x-2">
          <Back />
          <div>
            <span className="font-semibold">Easy returns</span>
            <p className="max-w-sm">
              Just return your product and we&apos;ll refund your money. No
              questions asked – we&apos;ll do our best to make sure your return
              is hassle-free.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}

export default ProductTabs
