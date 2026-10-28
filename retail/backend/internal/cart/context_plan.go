package cart

import "strings"

// ContextField distinguishes an omitted field from an explicitly cleared one.
// These are internal workflow values, not an HTTP request or an ownership grant.
type ContextField[T any] struct {
	Present bool
	Value   *T
}

type ContextAddress map[string]*string

type ContextRegion struct {
	ID, Currency string
	Countries    []string
}

type ContextState struct {
	Region  ContextRegion
	Locale  *string
	Address ContextAddress
}

type ContextChange struct {
	RegionID string // empty means omitted, as in the original region lookup
	Locale   ContextField[string]
	Address  ContextField[ContextAddress]
}

// ContextPlan is preparation only. A caller must resolve the target region
// from trusted storage and implement a complete atomic refresh before exposing
// a mutation: prices, taxes, promotions, shipping and custom-price deletion.
// No HTTP endpoint or enabled capability is provided by this planner.
type ContextPlan struct {
	RegionID, Currency string
	Locale             ContextField[string]
	Address            ContextField[ContextAddress]
	RegionChanged      bool
	RemoveCustomPrices bool
	ForceRefresh       bool
}

func copyAddress(in ContextAddress) ContextAddress {
	if in == nil {
		return nil
	}
	out := make(ContextAddress, len(in))
	for key, value := range in {
		if value != nil {
			v := *value
			out[key] = &v
		} else {
			out[key] = nil
		}
	}
	return out
}

func copyField[T any](in ContextField[T]) ContextField[T] {
	out := ContextField[T]{Present: in.Present}
	if in.Present && in.Value != nil {
		v := *in.Value
		out.Value = &v
	}
	return out
}

func equalNullable(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// PlanContext follows the installed update-cart workflow's region/address,
// locale and refresh decisions. Authorization, directory membership, version,
// idempotency and persistence belong to the enclosing command transaction.
func PlanContext(current ContextState, change ContextChange, target ContextRegion) (ContextPlan, error) {
	region := current.Region
	if change.RegionID != "" {
		if target.ID != change.RegionID {
			return ContextPlan{}, ErrRegion
		}
		region = target
	}
	if !idPattern.MatchString(region.ID) || !currencyPattern.MatchString(region.Currency) {
		return ContextPlan{}, ErrRegion
	}
	plan := ContextPlan{RegionID: region.ID, Currency: region.Currency, Locale: copyField(change.Locale)}
	plan.RegionChanged = change.RegionID != "" && change.RegionID != current.Region.ID
	plan.RemoveCustomPrices = plan.RegionChanged
	plan.Address.Present = change.Address.Present
	if change.Address.Present && change.Address.Value != nil {
		address := copyAddress(*change.Address.Value)
		plan.Address.Value = &address
		if country := address["country_code"]; country != nil && *country != "" {
			found := false
			for _, candidate := range region.Countries {
				found = found || candidate == *country
			}
			if !found {
				return ContextPlan{}, ErrRegion
			}
		}
	}
	// Validate an explicit country before single-country replacement. This order
	// matters: an invalid explicit address must fail rather than be overwritten.
	if plan.RegionChanged {
		plan.Address.Present = true
		if len(region.Countries) == 1 {
			country := region.Countries[0]
			address := ContextAddress{"country_code": &country}
			plan.Address.Value = &address
		}
		if plan.Address.Value == nil || (*plan.Address.Value)["country_code"] == nil || *(*plan.Address.Value)["country_code"] == "" {
			plan.Address.Value = nil
		}
	}
	addressChanged := false
	if change.Address.Present && change.Address.Value != nil {
		for _, key := range []string{"id", "country_code", "province", "postal_code", "city"} {
			next, present := (*change.Address.Value)[key]
			if !present {
				continue
			}
			prev := current.Address[key]
			if key == "country_code" {
				// JS optional chaining maps null and undefined countries to the
				// same value; other explicit null address fields remain changes.
				if next == nil && prev == nil {
					continue
				}
				if next != nil && prev != nil && strings.EqualFold(*next, *prev) {
					continue
				}
			} else if prev == nil && next == nil {
				_, existed := current.Address[key]
				if !existed { // explicit null differs from undefined
					addressChanged = true
				}
				continue
			}
			addressChanged = addressChanged || !equalNullable(next, prev)
		}
	}
	// The original refresh uses !!newLocaleCode: clearing a locale to null or
	// an empty string does not independently force a refresh.
	localeChanged := change.Locale.Present && !equalNullable(change.Locale.Value, current.Locale)
	plan.ForceRefresh = change.RegionID != "" || localeChanged && change.Locale.Value != nil && *change.Locale.Value != "" || addressChanged
	return plan, nil
}
