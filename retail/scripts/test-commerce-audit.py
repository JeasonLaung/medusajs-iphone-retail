"""Artificial financial-audit checks. No snapshot, database, account or provider access."""
import copy
import unittest
from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path

spec = spec_from_file_location('commerce_audit', Path(__file__).with_name('audit-commerce-snapshot.py'))
module = module_from_spec(spec)
spec.loader.exec_module(module)


class AuditTests(unittest.TestCase):
    def setUp(self):
        self.tables = {name: [] for name in ['capture', 'refund', 'payment', 'payment_session', 'payment_collection', 'order', 'order_item', 'order_summary']}
        self.tables['payment'] = [{'id': 'p', 'amount': '100', 'currency_code': 'eur', 'payment_collection_id': 'pc', 'payment_session_id': 'ps'}]
        self.tables['payment_session'] = [{'id': 'ps', 'currency_code': 'eur', 'payment_collection_id': 'pc'}]
        self.tables['payment_collection'] = [{'id': 'pc', 'currency_code': 'eur', 'captured_amount': '60', 'refunded_amount': '20'}]
        self.tables['capture'] = [{'id': 'c', 'payment_id': 'p', 'amount': '60'}]
        self.tables['refund'] = [{'id': 'r', 'payment_id': 'p', 'amount': '20'}]
        self.mapping = {'tables': [], 'fields': []}

    def audit(self):
        before = copy.deepcopy(self.tables)
        result = module.audit(self.tables, self.mapping)
        self.assertEqual(self.tables, before, 'Audit must not mutate source rows')
        return result

    def test_partial_capture_and_refund_are_valid(self):
        self.assertEqual(self.audit()['financial_or_version_findings'], {})

    def test_over_capture_is_reported(self):
        self.tables['capture'].append({'id': 'c2', 'payment_id': 'p', 'amount': '41'})
        self.assertEqual(self.audit()['financial_or_version_findings']['payment:captured_exceeds_amount'], 1)

    def test_refund_above_actual_capture_is_reported(self):
        self.tables['refund'][0]['amount'] = '61'
        self.assertEqual(self.audit()['financial_or_version_findings']['payment:refunded_exceeds_captured'], 1)

    def test_soft_deleted_capture_does_not_count_as_money_received(self):
        self.tables['capture'][0]['deleted_at'] = '2026-01-01T00:00:00Z'
        result = self.audit()['financial_or_version_findings']
        self.assertEqual(result['payment:refunded_exceeds_captured'], 1)
        self.assertEqual(result['payment_collection:captured_aggregate_mismatch'], 1)

    def test_collection_and_session_context_must_match(self):
        self.tables['payment_collection'][0]['currency_code'] = 'usd'
        self.tables['payment_session'][0]['payment_collection_id'] = 'other'
        result = self.audit()['financial_or_version_findings']
        self.assertEqual(result['payment:collection_currency_mismatch'], 1)
        self.assertEqual(result['payment:session_context_mismatch'], 1)

    def test_decimal_balances_do_not_use_float_rounding(self):
        self.tables['payment'][0]['amount'] = '0.3'
        self.tables['capture'] = [{'id': 'c1', 'payment_id': 'p', 'amount': '0.1'}, {'id': 'c2', 'payment_id': 'p', 'amount': '0.2'}]
        self.tables['refund'][0]['amount'] = '0.3'
        self.tables['payment_collection'][0].update(captured_amount='0.3', refunded_amount='0.3')
        self.assertEqual(self.audit()['financial_or_version_findings'], {})

    def test_negative_refund_is_reported(self):
        self.tables['refund'][0]['amount'] = '-1'
        self.assertEqual(self.audit()['financial_or_version_findings']['refund:negative_amount'], 1)

    def test_duplicate_current_version_and_missing_summary(self):
        self.tables['order'] = [{'id': 'o', 'version': 2}]
        self.tables['order_item'] = [{'id': 'oi1', 'order_id': 'o', 'version': 2, 'item_id': 'line'}, {'id': 'oi2', 'order_id': 'o', 'version': 2, 'item_id': 'line'}]
        result = self.audit()['financial_or_version_findings']
        self.assertEqual(result['order:current_version_summary_not_unique'], 1)
        self.assertEqual(result['order_item:duplicate_order_version_item'], 1)

    def test_unresolved_reference_is_retained_as_failure(self):
        self.tables['order_fulfillment'] = [{'id': 'link', 'fulfillment_id': 'missing', 'deleted_at': None}]
        self.tables['fulfillment'] = []
        self.mapping = {'tables': [{'source_table': 'fulfillment', 'target_table': 'fulfillments'}],
                        'fields': [{'source_table': 'order_fulfillment', 'source_column': 'fulfillment_id', 'reference': 'fulfillments.id'}]}
        result = self.audit()
        self.assertEqual(result['unresolved_references'][0]['not_soft_deleted_rows'], 1)
        self.assertEqual(result['distinct_rows_with_unresolved_references'], {'order_fulfillment': 1})


if __name__ == '__main__':
    unittest.main()
