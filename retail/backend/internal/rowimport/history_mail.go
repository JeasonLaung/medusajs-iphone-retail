package rowimport

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/eurorefurb/retail/internal/importvalue"
)

type EffectDisposition struct {
	SourceTable string
	SourceID    string
	Effect      string
	Disposition string
	Reason      string
}

// HistoricalMail returns BOTH the bound row and mandatory delivery disposition.
// A future importer must persist them atomically before any worker can run.
// No source status/timing is rewritten to pretend delivery or cancellation.
type HistoricalMail struct {
	Row      BoundRow
	Delivery EffectDisposition
}

var verificationMailID = regexp.MustCompile(`^mail_verify_[0-9a-f]{64}$`)

func ConvertHistoricalMail(row SourceRow) (HistoricalMail, error) {
	// Validate the complete source before any redaction can hide corrupt input.
	if _, err := convertRow("mail_message", row, nil, true); err != nil {
		return HistoricalMail{}, err
	}
	id, err := importvalue.Text(row["id"].Raw, 191)
	if err != nil || id == "" {
		return HistoricalMail{}, ErrHistoryPolicy
	}
	creator, _ := importvalue.Text(row["created_by"].Raw, 191)
	isRegistration := creator == "registration-verification"
	if strings.HasPrefix(id, "mail_verify_") && !isRegistration {
		return HistoricalMail{}, ErrHistoryPolicy
	}
	copyRow := make(SourceRow, len(row))
	for key, value := range row {
		copyRow[key] = value
	}
	disposition := EffectDisposition{SourceTable: "mail_message", SourceID: id, Effect: "mail-delivery", Disposition: "hold", Reason: "legacy-delivery-reconciliation"}
	if isRegistration {
		if !verificationMailID.MatchString(id) {
			return HistoricalMail{}, ErrHistoryPolicy
		}
		direction, _ := importvalue.Text(row["direction"].Raw, 0)
		if direction != "outbound" {
			return HistoricalMail{}, ErrHistoryPolicy
		}
		// Authentication bodies, attachment payloads and provider errors may
		// contain old links/codes. Keep their restricted source evidence only.
		for _, column := range []string{"text_body", "html_body", "error"} {
			copyRow[column] = Cell{Raw: json.RawMessage(`""`)}
		}
		copyRow["attachments"] = Cell{Raw: json.RawMessage(`[]`)}
		// The source generator uses only these two fixed subjects. An unknown
		// subject is held for explicit review rather than copied as a secret.
		subject, _ := importvalue.Text(row["subject"].Raw, 0)
		if subject != "验证邮箱，完成 EuroRefurb 注册" && subject != "Verify your email to join EuroRefurb" {
			return HistoricalMail{}, ErrHistoryPolicy
		}
		disposition.Disposition = "suppress-permanently"
		disposition.Reason = "excluded-legacy-registration-capability"
	}
	bound, err := convertRow("mail_message", copyRow, nil, true)
	if err != nil {
		return HistoricalMail{}, err
	}
	return HistoricalMail{Row: bound, Delivery: disposition}, nil
}
