export const getSearchCopy = (segment: string) => segment?.endsWith("-zh")
  ? { placeholder: "搜索商品", clear: "清除搜索" }
  : { placeholder: "Search products", clear: "Clear search" }
