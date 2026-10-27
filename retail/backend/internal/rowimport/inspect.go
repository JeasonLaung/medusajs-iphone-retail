package rowimport

// ValueInspection contains fixed mapping names and booleans/counts only.
// It has no source IDs, values, hashes, SQL arguments or authorization grants.
// A protected field does not stop checks of later ordinary fields or CHECKs.
type ValueInspection struct {
	Fields                int      `json:"fields"`
	SQLNulls              int      `json:"sql_nulls"`
	InvalidColumns        []string `json:"invalid_columns"`
	ProtectedColumns      []string `json:"protected_columns"`
	NumberPairsValid      bool     `json:"number_pairs_valid"`
	SourceChecksValid     bool     `json:"source_checks_valid"`
	RequiresHistoryPolicy bool     `json:"requires_history_policy"`
	OrdinaryConvertible   bool     `json:"ordinary_convertible"`
}

// InspectValues audits every mapped field without returning transformed data.
// This is diagnostic evidence, never a bypass of Convert, a domain-specific
// history/protection policy, cross-table reconciliation or full import gates.
// Invalid table/column coverage returns no partial inspection. All other
// failures are reported together, so a preceding secret cannot mask bad money.
func InspectValues(table string, row SourceRow) (ValueInspection, error) {
	plans, err := compiledPlans()
	if err != nil {
		return ValueInspection{}, ErrRow
	}
	p, exists := plans[table]
	if !exists || len(row) != len(p.Fields) {
		return ValueInspection{}, ErrRow
	}
	for _, f := range p.Fields {
		if _, exists := row[f.Source]; !exists {
			return ValueInspection{}, ErrRow
		}
	}
	r := ValueInspection{Fields: len(p.Fields), InvalidColumns: []string{}, ProtectedColumns: []string{},
		RequiresHistoryPolicy: table == "mail_message" || table == "payment_session" || table == "payment" || table == "account_holder"}
	for _, f := range p.Fields {
		cell := row[f.Source]
		if cell.SQLNull {
			r.SQLNulls++
			if !f.Nullable || len(cell.Raw) != 0 {
				r.InvalidColumns = append(r.InvalidColumns, f.Source)
			}
			continue
		}
		_, err := convertValue(f, cell.Raw)
		switch err {
		case nil:
		case ErrProtection:
			r.ProtectedColumns = append(r.ProtectedColumns, f.Source)
		default:
			r.InvalidColumns = append(r.InvalidColumns, f.Source)
		}
	}
	r.NumberPairsValid = checkNumberPairs(p.RawPairs, row) == nil
	r.SourceChecksValid = checkSourceConstraints(p.Checks, row) == nil
	r.OrdinaryConvertible = len(r.InvalidColumns) == 0 && len(r.ProtectedColumns) == 0 && r.NumberPairsValid && r.SourceChecksValid && !r.RequiresHistoryPolicy
	return r, nil
}
