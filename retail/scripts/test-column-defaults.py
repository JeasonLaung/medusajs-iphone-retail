"""Independent artificial expectations for default conversion policy boundaries."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('default_design', Path(__file__).with_name('map-column-defaults.py'))
design = importlib.util.module_from_spec(spec)
spec.loader.exec_module(design)


def column(default, table='fixture', name='value', **kwargs):
    return {'source_table': table, 'source_column': name, 'column_default': default,
            'is_identity': 'NO', 'is_generated': 'NEVER', **kwargs}


class DefaultsTests(unittest.TestCase):
    def test_excluded_identity_has_no_target_default(self):
        self.assertEqual(design.classify(column('now()'), False)[0], 'not-imported')

    def test_missing_is_not_implicit_zero(self):
        policy, value, _ = design.classify(column(None), True)
        self.assertEqual((policy, value), ('no-source-default', None))

    def test_singleton_key_is_boolean_not_random_id(self):
        for table in ['order_number_clock', 'sku_number_counter']:
            self.assertEqual(design.classify(column('true', table, 'id'), True)[:2], ('singleton-boolean-key', True))
            with self.assertRaises(ValueError):
                design.classify(column('false', table, 'id'), True)

    def test_numeric_default_keeps_lexeme(self):
        self.assertEqual(design.classify(column('9007199254740993.001'), True)[:2], ('exact-number-literal', '9007199254740993.001'))

    def test_json_not_double_encoded_or_float_parsed(self):
        value = '{"amount":9007199254740993.001}'
        self.assertEqual(design.classify(column("'" + value + "'::jsonb"), True)[:2], ('exact-json-literal', value))

    def test_postgres_quote_unescape(self):
        self.assertEqual(design.classify(column("'can''t'::text"), True)[:2], ('text-or-enum-literal', "can't"))

    def test_empty_array_not_brace_string(self):
        self.assertEqual(design.classify(column("'{}'::text[]"), True)[:2], ('empty-text-array', []))
        with self.assertRaises(ValueError):
            design.classify(column("'{a}'::text[]"), True)

    def test_sequence_is_application_counter(self):
        self.assertEqual(design.classify(column("nextval('fixture_seq'::regclass)"), True)[0], 'persistent-sequence-command')

    def test_unknown_function_and_cast_rejected(self):
        for value in ['uuid_generate_v4()', "'x'::unknown_enum", '1e2']:
            with self.assertRaises(ValueError):
                design.classify(column(value), True)

    def test_generated_column_requires_review(self):
        with self.assertRaises(ValueError):
            design.classify(column('0', is_generated='ALWAYS'), True)


if __name__ == '__main__':
    unittest.main()
