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
            ('deleted_at', 'DATETIME(6)'), ('is_exclusive', 'BOOLEAN'), ('note', 'LONGTEXT'),
            ('status', 'VARCHAR(32) utf8mb4_0900_bin'))}

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

    def test_nonunique_plain_text_prefix_is_explicit(self):
        sql, status, reason = self.convert(keys='note, id', unique=False)
        self.assertEqual(status, 'emitted')
        self.assertIn('(`note`(128),`id`)', sql[0])
        self.assertIn('full-value SQL predicate required', reason)

    def test_descending_index_direction_kept(self):
        sql, status, _ = self.convert(keys='note, deleted_at DESC', unique=False)
        self.assertEqual(status, 'emitted')
        self.assertIn('`deleted_at` DESC', sql[0])

    def test_json_and_encrypted_text_not_prefix_indexed(self):
        for design in ('LONGTEXT utf8mb4_0900_bin + CHECK JSON_VALID', 'LONGTEXT utf8mb4_0900_bin encrypted envelope', 'LONGBLOB (versioned authenticated envelope)'):
            with self.subTest(design=design):
                self.fields['note']['target_type_design'] = design
                self.assertEqual(self.convert(keys='note', unique=False)[1], 'hold')

    def test_expression_not_guessed(self):
        self.assertEqual(self.convert(keys='lower(handle)')[1], 'hold')

    def test_unknown_predicate_not_guessed(self):
        self.assertEqual(self.convert(where="(handle <> '')")[1], 'hold')

    def test_enum_predicate_preserves_all_participating_states(self):
        sql, status, _ = self.convert(where="((status = ANY (ARRAY['pending'::text, 'processing'::text])) AND (deleted_at IS NULL))")
        self.assertEqual(status, 'emitted')
        self.assertIn("`status` IN ('pending','processing') AND `deleted_at` IS NULL", sql[0])

    def test_single_text_predicate(self):
        sql, status, _ = self.convert(where="(status = 'failed'::text)")
        self.assertEqual(status, 'emitted')
        self.assertIn("`status` = 'failed'", sql[0])

    def test_nested_and_parentheses(self):
        self.assertEqual(self.convert(where='(((deleted_at IS NULL))) AND ((is_exclusive = false))')[1], 'emitted')

    def test_unknown_grammar_fails_closed(self):
        for where in ("(status = ANY (ARRAY[]))", "(status = ANY (ARRAY['pending'::varchar]))",
                      "(status = ANY (ARRAY['pending'::text, NULL::text]))", '(status IS NULL) OR (deleted_at IS NULL)',
                      '(deleted_at IS NULL))', '(deleted_at IS NULL', '(is_exclusive = true) trailing',
                      "(status = 'pending'::text); DROP TABLE fixture", '(status = false)',
                      "(is_exclusive = 'pending'::text)", '(unknown IS NULL)', "(status = lower('pending'))"):
            with self.subTest(where=where):
                self.assertEqual(self.convert(where=where)[1], 'hold')

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
