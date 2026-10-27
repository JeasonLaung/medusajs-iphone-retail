"""Artificial filesystem boundary cases; never touches the old asset roots."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('file_audit', Path(__file__).with_name('audit-file-snapshot.py'))
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)


class FileAuditTests(unittest.TestCase):
    def test_path_traversal_and_nonportable_keys(self):
        for value in [None, '', '../x', 'a/../x', 'a/./b', '/absolute', 'a//b', 'C:asset',
                      'C:/asset', 'a\\b', 'a\x00b', 'a\nb', 'a/', 'photo.jpg:stream',
                      'photo?.jpg', 'photo.', 'photos /a']:
            with self.subTest(value=value):
                self.assertIsNone(audit.safe_key(value))

    def test_valid_nested_key_preserves_case(self):
        self.assertEqual(audit.safe_key('photos/Abc.webp'), 'photos/Abc.webp')

    def test_inventory_hashes_without_mutating_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            folder = Path(tmp)
            (folder / 'nested').mkdir()
            file = folder / 'nested/image.bin'
            file.write_bytes(b'artificial-fixture')
            before = file.stat()
            result = audit.inventory(folder)
            self.assertEqual(set(result), {'nested/image.bin'})
            self.assertEqual(result['nested/image.bin']['bytes'], 18)
            self.assertEqual(result['nested/image.bin']['sha256'], audit.sha(file))
            self.assertEqual(file.stat().st_mtime_ns, before.st_mtime_ns)
            self.assertEqual(file.read_bytes(), b'artificial-fixture')

    def test_image_header_only_not_full_decode_claim(self):
        with tempfile.TemporaryDirectory() as tmp:
            file = Path(tmp) / 'photo'
            for content, expected in [(b'\xff\xd8\xff', 'image/jpeg'),
                                      (b'\x89PNG\r\n\x1a\n', 'image/png'),
                                      (b'RIFF\x00\x00\x00\x00WEBP', 'image/webp'),
                                      (b'not-image', None), (b'RIFF', None)]:
                file.write_bytes(content)
                self.assertEqual(audit.image_mime(file), expected)

    def test_linked_file_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            folder = Path(tmp)
            (folder / 'source').write_bytes(b'artificial')
            try:
                (folder / 'link').symlink_to(folder / 'source')
            except OSError:
                self.skipTest('OS denies synthetic symlink creation')
            with self.assertRaisesRegex(ValueError, 'Linked storage file'):
                audit.inventory(folder)


if __name__ == '__main__':
    unittest.main()
