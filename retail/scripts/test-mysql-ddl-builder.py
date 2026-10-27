"""Conversion counterexamples; does not substitute for actual MySQL execution."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('ddl_builder', Path(__file__).with_name('build-mysql-ddl.py'))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class IndexConversion(unittest.TestCase):
    def setUp(self):
        self.fields = {name: {'target_column': name, 'target_type_design': design} for name, design in (
            ('id', 'VARCHAR(191) utf8mb4_0900_bin'), ('handle', 'VARCHAR(512) utf8mb4_0900_bin'),
            ('deleted_at', 'DATETIME(6)'), ('is_exclusive', 'BOOLEAN'), ('note', 'LONGTEXT'))}

    def convert(self, keys='handle', where='(deleted_at IS NULL)', unique=True):
        return builder.index_sql({'index_name': 'fixture_unique', 'definition':
            'CREATE ' + ('UNIQUE ' if unique else '') + 'INDEX fixture_unique ON public.fixture USING btree (' + keys + ')' + (' WHERE ' + where if where else '')}, self.fields, 'target_fixture')

    def test_active_unique_uses_exact_value_and_null_flag(self):
        sql, status, _ = self.convert()
        self.assertEqual(status, 'emitted')
        self.assertIn('IF(`deleted_at` IS NULL,1,NULL)', sql[0])
        self.assertIn('(`handle`,', sql[1])
        self.assertNotIn('handle`(', sql[1])

    def test_multi_condition_boolean(self):
        sql, status, _ = self.convert(where='((deleted_at IS NULL) AND (is_exclusive = false))')
        self.assertEqual(status, 'emitted')
        self.assertIn('`deleted_at` IS NULL AND `is_exclusive` = 0', sql[0])

    def test_no_predicate_needs_no_generated_flag(self):
        sql, status, _ = self.convert(where=None)
        self.assertEqual(status, 'emitted')
        self.assertEqual(len(sql), 1)

    def test_nullable_composite_kept_uncoalesced(self):
        sql, status, _ = self.convert(keys='id, handle')
        self.assertEqual(status, 'emitted')
        self.assertIn('(`id`,`handle`,', sql[1])
        self.assertNotIn('COALESCE', ''.join(sql))

    def test_overwide_key_does_not_truncate(self):
        sql, status, _ = self.convert(keys='handle, handle')
        self.assertEqual(status, 'hold')
        self.assertEqual(sql, [])

    def test_longtext_does_not_use_prefix(self):
        self.assertEqual(self.convert(keys='note')[1], 'hold')

    def test_expression_not_guessed(self):
        self.assertEqual(self.convert(keys='lower(handle)')[1], 'hold')

    def test_unknown_predicate_not_guessed(self):
        self.assertEqual(self.convert(where="(handle <> '')")[1], 'hold')

    def test_nonunique_predicate_explicitly_broadened(self):
        sql, status, reason = self.convert(unique=False)
        self.assertEqual(status, 'emitted')
        self.assertEqual(len(sql), 1)
        self.assertIn('broadened', reason)

    def test_unrecognized_type_and_identifier_rejected(self):
        with self.assertRaises(ValueError):
            builder.sql_type('TEXT COLLATE random')
        with self.assertRaises(ValueError):
            builder.ident('value` OR true')


if __name__ == '__main__':
    unittest.main()
