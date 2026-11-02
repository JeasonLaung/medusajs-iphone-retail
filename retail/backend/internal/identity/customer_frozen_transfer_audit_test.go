package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func transferTableFingerprint(db *gorm.DB) ([32]byte, error) {
	var tables, all []string
	if err := db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY table_name").Scan(&tables).Error; err != nil {
		return [32]byte{}, err
	}
	for _, table := range tables {
		var rows []map[string]any
		if err := db.Table(table).Find(&rows).Error; err != nil {
			return [32]byte{}, err
		}
		var values []string
		for _, row := range rows {
			raw, err := json.Marshal(row)
			if err != nil {
				return [32]byte{}, err
			}
			values = append(values, string(raw))
		}
		sort.Strings(values)
		all = append(all, table+strings.Join(values, "\n"))
	}
	return sha256.Sum256([]byte(strings.Join(all, "\n"))), nil
}

func auditFirstPaymentAttachment(ctx context.Context, db *gorm.DB, accounts *customer.Store, sessions *sessionstore.Store, ref sessionstore.CartReference, options cart.InitialPaymentOptions) error {
	login, err := accounts.Login(ctx, "context-buyer@example.invalid", "context-owned-customer-password")
	if err != nil {
		return err
	}
	defer sessions.Revoke(context.WithoutCancel(ctx), sessionstore.Customer, login.Session.Token)
	possession, err := sessions.IssueCart(ctx, ref)
	if err != nil {
		return err
	}
	defer sessions.RevokeCart(context.WithoutCancel(ctx), possession.Token)
	command := cart.CustomerTransferCommand{OperationID: uuid.NewString(), Version: "5"}
	transfer := func() (cart.ContactResult, error) {
		return accounts.TransferCart(ctx, login.Session.Token, possession.Token, "shop", command, options.ManualProviders, options)
	}
	assertRollback := func(want error) error {
		before, err := transferTableFingerprint(db)
		if err != nil {
			return err
		}
		_, failure := transfer()
		after, err := transferTableFingerprint(db)
		if err != nil || before != after || !errors.Is(failure, want) {
			return fmt.Errorf("first payment attachment failed rollback: %v %v", failure, err)
		}
		return nil
	}
	if err := db.Exec("CREATE TRIGGER first_attachment_receipt_fault BEFORE INSERT ON customer_cart_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned first attachment rollback'").Error; err != nil {
		return err
	}
	failed := assertRollback(customer.ErrUnavailable)
	if err := db.Exec("DROP TRIGGER first_attachment_receipt_fault").Error; err != nil {
		return err
	}
	if failed != nil {
		return failed
	}
	// This automatic offer is eligible only AFTER the new ownership is visible
	// inside the transfer's transaction. An anonymous pre-check would miss it.
	statements := []string{
		`INSERT INTO promotions(id,code,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at) VALUES('attachment-offer','ATTACHMENT_OFFER',TRUE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO promotion_application_methods(id,promotion_id,type,target_type,allocation,value,raw_value,created_at,updated_at) VALUES('attachment-method','attachment-offer','percentage','items','across',10,'{"value":"10","precision":100}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO promotion_rules(id,attribute,operator,created_at,updated_at) VALUES('attachment-rule','customer.email','eq',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO promotion_rule_values(id,promotion_rule_id,value,created_at,updated_at) VALUES('attachment-value','attachment-rule','context-buyer@example.invalid',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO promotion_order_rules(promotion_id,promotion_rule_id) VALUES('attachment-offer','attachment-rule')`,
	}
	for _, q := range statements {
		if err := db.Exec(q).Error; err != nil {
			return err
		}
	}
	failed = assertRollback(customer.ErrCartConflict)
	for _, q := range []string{"DELETE FROM promotion_order_rules WHERE promotion_id='attachment-offer'", "DELETE FROM promotion_rule_values WHERE id='attachment-value'", "DELETE FROM promotion_rules WHERE id='attachment-rule'", "DELETE FROM promotion_application_methods WHERE id='attachment-method'", "DELETE FROM promotions WHERE id='attachment-offer'"} {
		if err := db.Exec(q).Error; err != nil {
			return err
		}
	}
	if failed != nil {
		return failed
	}
	// Paid evidence already applied to this attempt must block first attachment.
	if err := db.Exec("UPDATE cart_checkout_attempts SET observed_status='paid' WHERE cart_id=?", ref.CartID).Error; err != nil {
		return err
	}
	failed = assertRollback(customer.ErrCartConflict)
	if err := db.Exec("UPDATE cart_checkout_attempts SET observed_status=NULL WHERE cart_id=?", ref.CartID).Error; err != nil {
		return err
	}
	return failed
}

// This audit runs only on the currently owned browser fixture, at the transfer
// boundary. It does not create orders, captures, reservations or notifications.
func auditFrozenCustomerTransfer(ctx context.Context, db *gorm.DB, accounts *customer.Store, sessions *sessionstore.Store, ref sessionstore.CartReference, providers []string) error {
	login, err := accounts.Login(ctx, "context-buyer@example.invalid", "context-owned-customer-password")
	if err != nil {
		return err
	}
	defer sessions.Revoke(context.WithoutCancel(ctx), sessionstore.Customer, login.Session.Token)
	possession, err := sessions.IssueCart(ctx, ref)
	if err != nil {
		return err
	}
	defer sessions.RevokeCart(context.WithoutCancel(ctx), possession.Token)
	var version uint64
	if err := db.Table("cart_ownership").Select("version").Where("cart_id=?", ref.CartID).Scan(&version).Error; err != nil {
		return err
	}
	command := cart.CustomerTransferCommand{OperationID: uuid.NewString(), Version: strconv.FormatUint(version, 10)}
	transfer := func() (cart.ContactResult, error) {
		return accounts.TransferCart(ctx, login.Session.Token, possession.Token, "shop", command, providers)
	}
	fingerprint := func(excludeReceipts bool) ([32]byte, error) {
		var tables, all []string
		if err := db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY table_name").Scan(&tables).Error; err != nil {
			return [32]byte{}, err
		}
		for _, table := range tables {
			if excludeReceipts && table == "customer_cart_operations" {
				continue
			}
			var rows []map[string]any
			if err := db.Table(table).Find(&rows).Error; err != nil {
				return [32]byte{}, err
			}
			var values []string
			for _, row := range rows {
				raw, err := json.Marshal(row)
				if err != nil {
					return [32]byte{}, err
				}
				values = append(values, string(raw))
			}
			sort.Strings(values)
			all = append(all, table+strings.Join(values, "\n"))
		}
		return sha256.Sum256([]byte(strings.Join(all, "\n"))), nil
	}
	before, err := fingerprint(false)
	if err != nil {
		return err
	}
	if err := db.Exec("CREATE TRIGGER frozen_transfer_receipt_fault BEFORE INSERT ON customer_cart_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned frozen transfer rollback'").Error; err != nil {
		return err
	}
	_, failed := transfer()
	if err := db.Exec("DROP TRIGGER frozen_transfer_receipt_fault").Error; err != nil {
		return err
	}
	after, err := fingerprint(false)
	if err != nil || !errors.Is(failed, customer.ErrUnavailable) || before != after {
		return fmt.Errorf("frozen transfer receipt rollback failed: %v %v", failed, err)
	}
	before, err = fingerprint(true)
	if err != nil {
		return err
	}
	var group sync.WaitGroup
	results, failures := make([]cart.ContactResult, 6), make([]error, 6)
	for i := range results {
		group.Add(1)
		go func(i int) { defer group.Done(); results[i], failures[i] = transfer() }(i)
	}
	group.Wait()
	fresh := 0
	for i, result := range results {
		if failures[i] != nil || result.Version != command.Version || result.CartID != ref.CartID {
			return fmt.Errorf("frozen transfer concurrent acknowledgment failed: %v", failures[i])
		}
		if !result.Replayed {
			fresh++
		}
	}
	after, err = fingerprint(true)
	if err != nil || fresh != 1 || before != after {
		return fmt.Errorf("frozen acknowledgment changed business facts or duplicated receipt: %v", err)
	}
	// Receipt replay must still authenticate the current account and reject a
	// different in-progress payment refresh instead of acknowledging past state.
	if err := db.Exec("UPDATE customer_accounts SET enabled=FALSE WHERE id=?", login.Customer.ID).Error; err != nil {
		return err
	}
	_, denied := transfer()
	if err := db.Exec("UPDATE customer_accounts SET enabled=TRUE WHERE id=?", login.Customer.ID).Error; err != nil {
		return err
	}
	if !errors.Is(denied, customer.ErrUnauthorized) {
		return fmt.Errorf("disabled account replay accepted: %v", denied)
	}
	if err := db.Exec("UPDATE cart_ownership SET payment_refresh_operation_id=? WHERE cart_id=?", uuid.NewString(), ref.CartID).Error; err != nil {
		return err
	}
	_, denied = transfer()
	if err := db.Exec("UPDATE cart_ownership SET payment_refresh_operation_id=NULL WHERE cart_id=?", ref.CartID).Error; err != nil {
		return err
	}
	if !errors.Is(denied, customer.ErrCartConflict) {
		return fmt.Errorf("in-progress refresh bypassed: %v", denied)
	}
	after, err = fingerprint(true)
	if err != nil || before != after {
		return fmt.Errorf("rejected replay changed business facts: %v", err)
	}
	return nil
}
