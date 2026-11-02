package rowimport

import (
	"sort"
	"strings"
)

// Layout contains only compiled schema identifiers, never source business
// values. Returned slices are copies, so callers cannot change conversion.
type Layout struct {
	Source, Target string
	Primary        []string
	Columns        []LayoutColumn
	References     []LayoutReference
}

type LayoutColumn struct {
	Source, Target string
	Decimal        bool
}

type LayoutReference struct{ Column, Parent, ParentColumn string }

func Layouts() ([]Layout, error) {
	plans, err := compiledPlans()
	if err != nil {
		return nil, err
	}
	result := make([]Layout, 0, len(plans))
	for _, p := range plans {
		layout := Layout{Source: p.Source, Target: p.Target, Primary: append([]string(nil), p.PrimaryKey...)}
		for _, f := range p.Fields {
			layout.Columns = append(layout.Columns, LayoutColumn{Source: f.Source, Target: f.Target, Decimal: strings.HasPrefix(f.TargetType, "DECIMAL(")})
		}
		for _, r := range p.References {
			layout.References = append(layout.References, LayoutReference{r.Column, r.ParentTable, r.ParentColumn})
		}
		result = append(result, layout)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Source < result[j].Source })
	return result, nil
}
