import { PRODUCT_INDEX_NAME, priceAttribute } from "../client"

export const OPTION_VALUES_ATTRIBUTE = "option_values"
export const CATEGORY_ATTRIBUTE = "category"
// The index calls the product's tags "labels".
export const LABELS_ATTRIBUTE = "labels"
export const MODEL_ATTRIBUTE = "model"
export const STORAGE_OPTIONS_ATTRIBUTE = "storage_options"
export const COLOR_OPTIONS_ATTRIBUTE = "color_families"
export const BATTERY_OPTIONS_ATTRIBUTE = "battery_options"
export const CONDITION_OPTIONS_ATTRIBUTE = "condition_options"
export const RELEASE_YEAR_ATTRIBUTE = "release_year"
export const DISPLAY_SIZE_ATTRIBUTE = "display_size"
export const REAR_CAMERAS_ATTRIBUTE = "rear_cameras"

export const getSortOptions = (currencyCode: string) => {
  const minPrice = priceAttribute("min_price", currencyCode)

  return [
    { value: PRODUCT_INDEX_NAME, label: "Relevance" },
    {
      value: `${PRODUCT_INDEX_NAME}/sort/created_at:desc`,
      label: "Latest Arrivals",
    },
    {
      value: `${PRODUCT_INDEX_NAME}/sort/${minPrice}:asc`,
      label: "Price: Low -> High",
    },
    {
      value: `${PRODUCT_INDEX_NAME}/sort/${minPrice}:desc`,
      label: "Price: High -> Low",
    },
    { value: `${PRODUCT_INDEX_NAME}/sort/title:asc`, label: "Title: A -> Z" },
    { value: `${PRODUCT_INDEX_NAME}/sort/title:desc`, label: "Title: Z -> A" },
  ]
}
