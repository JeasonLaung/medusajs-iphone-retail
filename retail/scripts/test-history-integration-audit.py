"""Adversarial number-clock checks; synthetic IDs and no database writes."""
import unittest
from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path

spec = spec_from_file_location('history_audit', Path(__file__).with_name('audit-history-integration-snapshot.py'))
module = module_from_spec(spec)
spec.loader.exec_module(module)


def fixture():
    return {'order_number_clock': [{'id': True, 'last_tick': 0}],
            'sku_number_counter': [{'id': True, 'last_id': 0}],
            'product_variant': [], 'order': [], 'workflow_execution': [],
            'script_migrations': [], 'paypal_webhook_event': [], 'notification': [], 'paypal_connection': []}


class ClockAudit(unittest.TestCase):
    def test_soft_deleted_variant_still_reserves_sku(self):
        data = fixture()
        data['product_variant'] = [{'sku': 'ERP10001', 'deleted_at': 'old'}]
        self.assertEqual(module.audit(data)[1], {'sku_number_counter:below_historical_allocated_sku': 1})

    def test_ten_microsecond_tick_and_shanghai_calendar(self):
        data = fixture()
        data['order'] = [{'custom_display_id': '1970010108000000001'}]
        self.assertEqual(module.audit(data)[1], {'order_number_clock:below_historical_number': 1})
        data['order_number_clock'][0]['last_tick'] = 1
        self.assertFalse(module.audit(data)[1])

    def test_legacy_format_and_invalid_calendar_differ(self):
        data = fixture()
        data['order'] = [{'custom_display_id': '123'}, {'custom_display_id': '2026130108000000001'}]
        _, issues, holds = module.audit(data)
        self.assertEqual(issues, {'order_number:invalid_calendar': 1})
        self.assertEqual(holds, {'order_number:legacy_format_requires_preservation': 1})

    def test_false_or_duplicate_singleton_cannot_allocate(self):
        data = fixture()
        data['order_number_clock'] = [{'id': False, 'last_tick': 1}]
        data['sku_number_counter'] *= 2
        self.assertEqual(len(module.audit(data)[1]), 2)

    def test_pending_work_is_held_without_claiming_settlement(self):
        data = fixture()
        data['paypal_webhook_event'] = [{'attempt_count': 0, 'status': 'processing'}]
        data['workflow_execution'] = [{'state': 'running'}]
        _, issues, holds = module.audit(data)
        self.assertFalse(issues)
        self.assertEqual(holds, {'paypal_webhook_event:reconcile_before_retry': 1,
                                'workflow_execution:manual_side_effect_reconciliation': 1})


if __name__ == '__main__':
    unittest.main()
