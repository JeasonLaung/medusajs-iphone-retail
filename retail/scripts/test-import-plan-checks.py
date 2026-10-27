"""CHECK compiler boundaries; metadata only, no business data or SQL execution."""
import importlib.util
import json
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


class RelationshipCompilerTests(unittest.TestCase):
    def fixture(self):
        plans = json.loads(compiler.OUT.read_text(encoding='utf-8'))['tables']
        fields = []
        for name in compiler.DOMAINS:
            fields.extend(json.loads((compiler.DOC / (name + '-field-mapping.json')).read_text(encoding='utf-8'))['fields'])
        constraints = json.loads((compiler.DOC / 'source-schema-constraints.json').read_text(encoding='utf-8'))['constraints']
        return plans, fields, constraints

    def test_all_pinned_references_and_primary_keys(self):
        plans, fields, constraints = self.fixture()
        compiler.relationships(plans, fields, constraints)
        self.assertEqual(sum(len(p['references']) for p in plans), 224)
        self.assertTrue(all(p['primary_key'] for p in plans))

    def test_mapping_cannot_disagree_with_physical_fk(self):
        plans, fields, constraints = self.fixture()
        next(f for f in fields if f['source_table'] == 'product' and f['source_column'] == 'collection_id')['reference'] = 'catalog_product_types.id'
        with self.assertRaises(AssertionError):
            compiler.relationships(plans, fields, constraints)

    def test_unsupported_foreign_key_or_parent_refused(self):
        for change in ('FOREIGN KEY (collection_id, type_id) REFERENCES product_collection(id, id)',
                       'FOREIGN KEY (collection_id) REFERENCES product_collection(id) MATCH FULL',
                       'FOREIGN KEY (collection_id) REFERENCES missing_table(id)'):
            plans, fields, constraints = self.fixture()
            next(c for c in constraints if c['constraint_name'] == 'product_collection_id_foreign')['definition'] = change
            with self.subTest(change=change), self.assertRaises(AssertionError):
                compiler.relationships(plans, fields, constraints)

    def test_nonunique_parent_and_nullable_primary_key_refused(self):
        plans, fields, constraints = self.fixture()
        next(c for c in constraints if c['source_table'] == 'product_collection' and c['constraint_type'] == 'p')['definition'] = 'PRIMARY KEY (id, handle)'
        with self.assertRaises(AssertionError):
            compiler.relationships(plans, fields, constraints)
        plans, fields, constraints = self.fixture()
        next(f for f in fields if f['source_table'] == 'product_tag' and f['source_column'] == 'id')['nullable'] = True
        with self.assertRaises(AssertionError):
            compiler.relationships(plans, fields, constraints)


if __name__ == '__main__':
    unittest.main()
