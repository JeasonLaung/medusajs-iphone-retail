package rowimport

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/eurorefurb/retail/internal/importvalue"
)

var (
	ErrReferences    = errors.New("snapshot contains unresolved references")
	ErrRelationLimit = errors.New("snapshot relationship audit exceeds configured limit")
)

type referenceRule struct {
	Column             string `json:"column"`
	ParentTable        string `json:"parent_table"`
	ParentColumn       string `json:"parent_column"`
	Policy             string `json:"policy"`
	PhysicalConstraint string `json:"physical_constraint"`
}

// KeyBytes bounds retained encoded-key/value payload, not the Go heap. MaxRows
// independently bounds maps and pending-entry overhead. Both must be explicit.
type RelationLimits struct{ MaxRows, KeyBytes int64 }

// Evidence contains only fixed table/column names and aggregate counts.
// MissingOnDeletedRows/HistoryPolicyMissing remain unresolved; neither grants
// an exception to relational acceptance or authorization for a new account.
type RelationEvidence struct {
	Rules                 int              `json:"rules"`
	PhysicalForeignKeys   int              `json:"physical_foreign_keys"`
	Rows                  int64            `json:"rows"`
	NullReferences        int64            `json:"null_references"`
	NonNullReferences     int64            `json:"nonnull_references"`
	ResolvedReferences    int64            `json:"resolved_references"`
	Missing               map[string]int64 `json:"missing"`
	PhysicalMissing       map[string]int64 `json:"physical_missing"`
	MissingOnDeletedRows  map[string]int64 `json:"missing_on_deleted_rows"`
	HistoryPolicyMissing  map[string]int64 `json:"history_policy_missing"`
	ResolvedToDeletedRows map[string]int64 `json:"resolved_to_deleted_rows"`
}

type RelationshipSnapshotEvidence struct {
	Snapshot  SnapshotEvidence
	Relations RelationEvidence
}

type pendingReference struct {
	name, value string
	rule        referenceRule
	deleted     bool
}
type relationCollector struct {
	plans    map[string]tablePlan
	fields   map[string]map[string]field
	primary  map[string]map[string]struct{}
	parents  map[string]map[string]bool
	needed   map[string][]string
	pending  []pendingReference
	limits   RelationLimits
	keyBytes int64
	evidence RelationEvidence
}

func newRelationCollector(limits RelationLimits) (*relationCollector, error) {
	if limits.MaxRows <= 0 || limits.KeyBytes <= 0 {
		return nil, ErrRelationLimit
	}
	plans, err := compiledPlans()
	if err != nil {
		return nil, ErrRow
	}
	c := &relationCollector{plans: plans, fields: map[string]map[string]field{}, primary: map[string]map[string]struct{}{}, parents: map[string]map[string]bool{}, needed: map[string][]string{}, limits: limits,
		evidence: RelationEvidence{Missing: map[string]int64{}, PhysicalMissing: map[string]int64{}, MissingOnDeletedRows: map[string]int64{}, HistoryPolicyMissing: map[string]int64{}, ResolvedToDeletedRows: map[string]int64{}}}
	for _, plan := range plans {
		if len(plan.PrimaryKey) == 0 {
			return nil, ErrRow
		}
		c.primary[plan.Source] = map[string]struct{}{}
		c.fields[plan.Source] = map[string]field{}
		for _, f := range plan.Fields {
			c.fields[plan.Source][f.Source] = f
		}
		for _, r := range plan.References {
			c.evidence.Rules++
			if r.PhysicalConstraint != "" {
				c.evidence.PhysicalForeignKeys++
			}
			index := r.ParentTable + "." + r.ParentColumn
			if _, exists := c.parents[index]; !exists {
				c.parents[index] = map[string]bool{}
				c.needed[r.ParentTable] = append(c.needed[r.ParentTable], r.ParentColumn)
			}
		}
	}
	if c.evidence.Rules != 224 || c.evidence.PhysicalForeignKeys != 88 {
		return nil, ErrRow
	}
	return c, nil
}

func (c *relationCollector) reserve(bytes int) error {
	if bytes < 0 || int64(bytes) > c.limits.KeyBytes-c.keyBytes {
		return ErrRelationLimit
	}
	c.keyBytes += int64(bytes)
	return nil
}

func (c *relationCollector) text(table, column string, row SourceRow) (string, bool, error) {
	f, exists := c.fields[table][column]
	cell, present := row[column]
	if !exists || !present {
		return "", false, ErrRow
	}
	if cell.SQLNull {
		if !f.Nullable || len(cell.Raw) != 0 {
			return "", false, ErrRow
		}
		return "", true, nil
	}
	switch f.Transform {
	case "identity_string", "code_string", "utf8_text":
	default:
		return "", false, ErrRow
	}
	value, err := importvalue.Text(cell.Raw, f.MaxChars)
	if err != nil {
		return "", false, ErrRow
	}
	return value, false, nil
}

func (c *relationCollector) add(table string, row SourceRow) error {
	plan, exists := c.plans[table]
	if !exists || len(row) != len(plan.Fields) {
		return ErrRow
	}
	for _, f := range plan.Fields {
		if _, exists := row[f.Source]; !exists {
			return ErrRow
		}
	}
	if c.evidence.Rows >= c.limits.MaxRows {
		return ErrRelationLimit
	}
	deleted := false
	if _, exists := c.fields[table]["deleted_at"]; exists {
		cell := row["deleted_at"]
		if cell.SQLNull {
			if len(cell.Raw) != 0 {
				return ErrRow
			}
		} else {
			if _, err := importvalue.Instant(cell.Raw); err != nil {
				return ErrRow
			}
			deleted = true
		}
	}
	var key []byte
	for _, column := range plan.PrimaryKey {
		f := c.fields[table][column]
		cell := row[column]
		if cell.SQLNull {
			return ErrRow
		}
		if f.Transform == "strict_boolean" {
			value, err := importvalue.Boolean(cell.Raw)
			if err != nil {
				return ErrRow
			}
			key = append(key, 'b', 0)
			if value {
				key[len(key)-1] = 1
			}
		} else {
			value, _, err := c.text(table, column, row)
			if err != nil {
				return ErrRow
			}
			key = append(key, 's')
			key = binary.BigEndian.AppendUint32(key, uint32(len(value)))
			key = append(key, value...)
		}
	}
	if err := c.reserve(len(key)); err != nil {
		return err
	}
	if _, duplicate := c.primary[table][string(key)]; duplicate {
		return ErrRow
	}
	c.primary[table][string(key)] = struct{}{}
	for _, column := range c.needed[table] {
		value, null, err := c.text(table, column, row)
		if err != nil || null {
			return ErrRow
		}
		if err := c.reserve(len(value)); err != nil {
			return err
		}
		c.parents[table+"."+column][value] = deleted
	}
	for _, rule := range plan.References {
		value, null, err := c.text(table, rule.Column, row)
		if err != nil {
			return err
		}
		if null {
			c.evidence.NullReferences++
			continue
		}
		if err := c.reserve(len(value)); err != nil {
			return err
		}
		c.evidence.NonNullReferences++
		c.pending = append(c.pending, pendingReference{name: table + "." + rule.Column, value: value, rule: rule, deleted: deleted})
	}
	c.evidence.Rows++
	return nil
}

func (c *relationCollector) finish() (RelationEvidence, error) {
	for _, ref := range c.pending {
		deleted, found := c.parents[ref.rule.ParentTable+"."+ref.rule.ParentColumn][ref.value]
		if found {
			c.evidence.ResolvedReferences++
			if deleted {
				c.evidence.ResolvedToDeletedRows[ref.name]++
			}
			continue
		}
		c.evidence.Missing[ref.name]++
		if ref.rule.PhysicalConstraint != "" {
			c.evidence.PhysicalMissing[ref.rule.PhysicalConstraint]++
		}
		if ref.deleted {
			c.evidence.MissingOnDeletedRows[ref.name]++
			if ref.rule.Policy == "preserve-unresolved-on-soft-deleted-row-only" {
				c.evidence.HistoryPolicyMissing[ref.name]++
			}
		}
	}
	if len(c.evidence.Missing) != 0 {
		return c.evidence, ErrReferences
	}
	return c.evidence, nil
}

// VerifyRelationships validates v2 framing/hash before accepting any relationship
// evidence. It checks all source primary keys and mapped/physical references,
// including forward/cyclic and soft-deleted targets. It does not Convert all
// values, decrypt secrets, apply unique claims, import rows or authorize owners.
// On ErrReferences the fully verified snapshot and aggregate diagnostics are
// returned for reconciliation; any other error returns empty evidence.
func VerifyRelationships(input io.Reader, expectedSHA256 string, limits RelationLimits) (RelationshipSnapshotEvidence, error) {
	c, err := newRelationCollector(limits)
	if err != nil {
		return RelationshipSnapshotEvidence{}, err
	}
	var callbackError error
	snapshot, err := VerifyStream(input, expectedSHA256, func(table string, row SourceRow) error {
		callbackError = c.add(table, row)
		return callbackError
	})
	if err != nil {
		if callbackError != nil {
			return RelationshipSnapshotEvidence{}, callbackError
		}
		return RelationshipSnapshotEvidence{}, ErrRow
	}
	relations, err := c.finish()
	return RelationshipSnapshotEvidence{Snapshot: snapshot, Relations: relations}, err
}
