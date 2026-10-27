"""Synthetic adversarial checks for custom-business migration diagnostics; no real data."""
import copy
import unittest
from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path

spec = spec_from_file_location('operations_audit', Path(__file__).with_name('audit-operations-snapshot.py'))
module = module_from_spec(spec)
spec.loader.exec_module(module)


def fixture():
    return {name: [] for name in ('buyback_request', 'buyback_photo', 'buyback_device_link',
                                 'mail_contact', 'mail_message', 'mail_delivery', 'mail_campaign',
                                 'seo_source', 'seo_monitor', 'seo_run', 'product_review')}


class Diagnostics(unittest.TestCase):
    def test_cross_request_photo_rejected_and_no_mutation(self):
        data = fixture()
        data['buyback_request'] = [{'id': 'r', 'items': [{'id': 'a'}], 'status': 'awaiting_photos'}]
        data['buyback_photo'] = [{'request_id': 'r', 'item_id': 'b', 'review_status': 'pending', 'storage_key': 'private'}]
        before = copy.deepcopy(data)
        _, issues, holds = module.audit(data)
        self.assertEqual(issues, {'buyback_photo:item_not_in_request': 1})
        self.assertEqual(holds['private_photo_files_require_manifest_and_checksum'], 1)
        self.assertEqual(data, before)

    def test_duplicate_nested_items_rejected(self):
        data = fixture()
        data['buyback_request'] = [{'id': 'r', 'items': [{'id': 'a'}, {'id': 'a'}], 'status': 'received'}]
        self.assertEqual(module.audit(data)[1]['buyback_request:invalid_or_duplicate_item_id'], 1)

    def test_closed_request_and_expired_photo_boundaries(self):
        data = fixture()
        data['buyback_request'] = [{'id': 'r', 'items': [{'id': 'a'}], 'status': 'paid', 'closed_at': None}]
        data['buyback_photo'] = [{'request_id': 'r', 'item_id': 'a', 'review_status': 'expired', 'storage_key': ''}]
        self.assertEqual(module.audit(data)[1], {'buyback_request:terminal_without_closed_at': 1})

    def test_suppression_cannot_be_treated_as_subscribed(self):
        data = fixture()
        data['mail_contact'] = [{'subscribed': True, 'suppressed': True, 'consent_source': 'form'}]
        self.assertEqual(module.audit(data)[1]['mail_contact:subscription_conflicts_with_suppression_or_consent'], 1)

    def test_pending_delivery_and_legacy_auth_mail_have_separate_holds(self):
        data = fixture()
        data['mail_message'] = [{'created_by': 'registration-verification', 'status': 'queued', 'attempts': 0}]
        data['mail_delivery'] = [{'status': 'sending', 'attempts': 1}]
        _, issues, holds = module.audit(data)
        self.assertFalse(issues)
        self.assertEqual(holds, {'legacy_registration_mail_never_replay': 1,
                                'mail_delivery:pending_work_reconciliation_hold': 1,
                                'mail_message:pending_work_reconciliation_hold': 1})

    def test_fractional_offset_rejected_large_integer_preserved(self):
        data = fixture()
        data['seo_source'] = [{'read_offset': '0.5', 'enabled': False},
                              {'read_offset': '9007199254740993', 'enabled': True}]
        _, issues, holds = module.audit(data)
        self.assertEqual(issues, {'seo_source:invalid_byte_offset': 1})
        self.assertEqual(holds, {'seo_source:host_path_and_fingerprint_review': 1})

    def test_boolean_rating_and_unverified_purchase_rejected(self):
        data = fixture()
        data['product_review'] = [{'rating': True, 'verified_purchase': True, 'order_id': None}]
        self.assertEqual(module.audit(data)[1], {'product_review:rating_out_of_range': 1,
                                               'product_review:verified_without_order_reference': 1})


if __name__ == '__main__':
    unittest.main()
