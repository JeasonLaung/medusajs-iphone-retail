import type { UiState } from "instantsearch.js"
import type { history } from "instantsearch.js/es/lib/routers"

const refinementParams: Record<string, string> = {
  category: "series",
  model: "model",
  storage_options: "storage",
  battery_options: "battery",
  display_size: "display",
  color_families: "color",
  rear_cameras: "camera",
  condition_options: "condition",
  release_year: "year",
  labels: "labels",
  option_values: "options",
}

type RouteState = Record<string, unknown>

export function createStoreSearchRouting(indexName: string) {
  const defaultSort = `${indexName}/sort/created_at:desc`
  return {
    router: {
      createURL({ qsModule, routeState, location }) {
        // Next.js may encode the separators before SSR. Escape commas inside
        // individual values separately so they remain distinguishable.
        const params = Object.fromEntries(
          Object.entries(routeState).map(([key, value]) => [
            key,
            Array.isArray(value)
              ? value.map((item) =>
                  typeof item === "string"
                    ? item.replace(/%/g, "%25").replace(/,/g, "%2C")
                    : item
                )
              : value,
          ])
        )
        const query = qsModule.stringify(params, {
          arrayFormat: "comma",
          encodeValuesOnly: true,
        })
        return `${location.origin}${location.pathname}${
          query ? `?${query}` : ""
        }${location.hash}`
      },
      parseURL({ qsModule, location }) {
        return qsModule.parse(location.search.slice(1), { comma: true })
      },
    } satisfies NonNullable<Parameters<typeof history<RouteState>>[0]>,
    stateMapping: {
      stateToRoute(uiState: UiState): RouteState {
        const { configure, refinementList, query, sortBy, ...rest } =
          uiState[indexName] ?? {}
        const route: RouteState = { ...rest }
        // InstantSearch omits sortBy when the base (relevance) index is selected.
        if (sortBy !== defaultSort) route.sortBy = sortBy || indexName
        if (query) route.q = query
        for (const [attribute, values] of Object.entries(
          refinementList ?? {}
        )) {
          if (values.length)
            route[refinementParams[attribute] ?? attribute] = values
        }
        return route
      },
      routeToState(route: RouteState): UiState {
        // Keep existing shared/bookmarked InstantSearch links working.
        if (route[indexName] && typeof route[indexName] === "object") {
          const legacy = {
            sortBy: defaultSort,
            ...(route[indexName] as UiState[string]),
          }
          if (legacy.page !== undefined) {
            const page = Number(legacy.page)
            if (Number.isInteger(page) && page > 0) legacy.page = page
            else delete legacy.page
          }
          return { [indexName]: legacy }
        }

        const state: UiState[string] = { sortBy: defaultSort }
        const refinementList: Record<string, string[]> = {}
        for (const [attribute, param] of Object.entries(refinementParams)) {
          const value = route[param]
          const values = Array.isArray(value) ? value : [value]
          const selected = values
            .filter(
              (item): item is string =>
                typeof item === "string" && item.length > 0
            )
            .flatMap((item) => item.split(","))
            .filter(Boolean)
            .map((item) =>
              item.replace(/%2C|%25/g, (escape) =>
                escape === "%2C" ? "," : "%"
              )
            )
          if (selected.length) refinementList[attribute] = selected
        }
        if (Object.keys(refinementList).length)
          state.refinementList = refinementList
        if (typeof route.q === "string") state.query = route.q
        const page = Number(route.page)
        if (Number.isInteger(page) && page > 1) state.page = page
        if (typeof route.sortBy === "string") state.sortBy = route.sortBy
        if (route.range && typeof route.range === "object") {
          state.range = route.range as UiState[string]["range"]
        }
        if (route.toggle && typeof route.toggle === "object") {
          state.toggle = route.toggle as UiState[string]["toggle"]
        }
        return { [indexName]: state }
      },
    },
  }
}
