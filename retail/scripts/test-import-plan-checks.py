"""CHECK compiler boundaries; metadata only, no business data or SQL execution."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('import_plans', Path(__file__).with_name('build-import-plans.py'))
compiler = importlib.util.module_from_spec(spec)
spec.loader.exec_module(compiler)


class CheckCompilerTests(unittest.TestCase):
    def compile(self, definition, fields):
        return compiler.source_check({'definition': definition, 'constraint_name': 'fixture'}, fields)

    def test_text_check_restricts_plain_text_mapping(self):
        result = self.compile("CHECK (status = ANY (ARRAY['draft'::text, 'active'::text]))", {'status': {'transform': 'utf8_text', 'allowed': []}})
        self.assertEqual(result['allowed'], ['draft', 'active'])
        self.assertEqual(result['columns'], ['status'])

    def test_enum_mapping_cannot_diverge(self):
        with self.assertRaises(AssertionError):
            self.compile("CHECK (status = ANY (ARRAY['draft'::text]))", {'status': {'transform': 'enum_text', 'allowed': ['active']}})

    def test_unreviewed_expressions_refused(self):
        for expression in ('CHECK (amount > 0::numeric)', 'CHECK (amount >= 1::numeric)', 'CHECK (id OR true)',
                           "CHECK (status = ANY (ARRAY['draft'::text])); DROP TABLE x", 'CHECK (lower(status) = \'draft\')'):
            with self.subTest(expression=expression), self.assertRaises(AssertionError):
                self.compile(expression, {})

    def test_unknown_enum_cast_or_literal_refused(self):
        for text in ("'draft'::varchar", "'draft'::text, NULL", "'draft'::text, 'MixedCase'::text"):
            with self.subTest(text=text), self.assertRaises(AssertionError):
                self.compile('CHECK (status = ANY (ARRAY[' + text + ']))', {'status': {'transform': 'utf8_text'}})

    def test_constraint_cannot_apply_to_wrong_type(self):
        for definition, fields in (
            ('CHECK (id)', {'id': {'transform': 'identity_string'}}),
            ('CHECK (amount >= 0::numeric)', {'amount': {'transform': 'finite_float32_to_float64'}}),
            ('CHECK (last_id >= 0)', {'last_id': {'transform': 'money_major_decimal'}}),
        ):
            with self.subTest(definition=definition), self.assertRaises(AssertionError):
                self.compile(definition, fields)

    def test_missing_constraint_field_refused(self):
        with self.assertRaises(AssertionError):
            self.compile('CHECK (parent_id IS NULL OR province_code IS NOT NULL)', {'parent_id': {'transform': 'identity_string'}})


if __name__ == '__main__':
    unittest.main()
