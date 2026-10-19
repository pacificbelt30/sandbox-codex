import io
from contextlib import redirect_stdout
import tarfile
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import setup


class SetupTests(unittest.TestCase):
    def test_existing_cxd_is_not_overwritten_by_first_named_upgrade(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'cxd'
            path.write_text('another program')
            with self.assertRaisesRegex(ValueError, 'unmanaged cxd command'):
                setup.check_new_cli_collision({'release': '/opt/old'}, path)
            setup.check_new_cli_collision({'release': '/opt/new', 'cxd_cli': True}, path)
            setup.check_new_cli_collision(None, path)

    def test_plan_shows_bundle_version_and_operator_before_apply(self):
        with tempfile.TemporaryDirectory() as tmp:
            bundle = Path(tmp)
            record = {'version': 'test-version'}
            output = io.StringIO()
            with patch('sys.argv', ['setup.py', '--bundle', str(bundle), '--operator', 'alice']), \
                    patch.object(setup, 'bundle_info', return_value=record), \
                    patch.object(setup, 'host_check'), \
                    redirect_stdout(output):
                setup.main()
            self.assertIn(f'Bundle: {bundle.resolve()} (version test-version); operator: alice', output.getvalue())
            self.assertIn('No host changes.', output.getvalue())

    def test_bundle_rejects_changed_helpers_and_extra_paths(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            files = {}
            for name in setup.BUNDLE_FILES:
                (root / name).write_bytes(b'trusted-fixture')
                files[name] = setup.sha(root / name)
            manifest = {'schema': 1, 'files': files}
            (root / 'bundle.json').write_text(json.dumps(manifest))
            setup.bundle_info(root)
            (root / setup.BUNDLE_FILES[1]).write_bytes(b'changed')
            with self.assertRaisesRegex(ValueError, 'checksum'):
                setup.bundle_info(root)
            manifest['files']['../outside'] = 'a' * 64
            (root / 'bundle.json').write_text(json.dumps(manifest))
            with self.assertRaisesRegex(ValueError, 'incomplete'):
                setup.bundle_info(root)

    def test_rollback_restores_contents_modes_and_absent_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            existing, absent = root / 'unit', root / 'cli'
            existing.write_bytes(b'previous service')
            existing.chmod(0o640)
            project = root / 'project'
            project.mkdir()
            (project / 'work').write_text('keep me')
            with patch.object(setup, 'MANAGED', [existing, absent]), patch.object(setup, 'trusted'):
                files = setup.backup_files(root / 'backup', [existing, absent])
                setup.atomic(existing, b'new service')
                setup.atomic(absent, b'new CLI', 0o755)
                setup.restore_files(files)
            self.assertEqual(existing.read_bytes(), b'previous service')
            self.assertEqual(existing.stat().st_mode & 0o777, 0o640)
            self.assertFalse(absent.exists())
            self.assertEqual((project / 'work').read_text(), 'keep me')

    def test_offline_rejects_missing_or_changed_archives(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            with self.assertRaisesRegex(ValueError, 'offline archive'):
                setup.fetch('kata', root, True)
            (root / setup.LOCK['kata']['file']).write_bytes(b'corrupt')
            with self.assertRaisesRegex(ValueError, 'checksum'):
                setup.fetch('kata', root, True)

    def test_units_only_target_dedicated_services(self):
        units = setup.units('/opt/codex-dock/releases/fixture')
        self.assertEqual(set(units), set(setup.MANAGED[:3]))
        self.assertIn('--manifest /opt/codex-dock/releases/fixture/kata-assets.json', ''.join(units.values()))
        self.assertNotIn('docker.service', ''.join(units.values()))
        self.assertIn('KillMode=process', ''.join(units.values()))

    def test_archive_does_not_apply_paths_or_executable_modes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            archive = root / 'payload.tar.gz'
            with tarfile.open(archive, 'w:gz') as tar:
                for name in ['../escape', '/absolute-escape', 'bin/tool']:
                    item = tarfile.TarInfo(name)
                    item.size = 3
                    item.mode = 0o4777
                    tar.addfile(item, io.BytesIO(b'abc'))
            destination = root / 'selected'
            setup.extract_selected(archive, {'bin/tool': (destination, 0o755)})
            self.assertEqual(destination.read_bytes(), b'abc')
            self.assertEqual(destination.stat().st_mode & 0o7777, 0o755)
            self.assertEqual({p.name for p in root.iterdir()}, {'selected', 'payload.tar.gz'})

    def test_unavailable_engine_is_not_empty(self):
        with patch.object(setup, 'SOCKET', '/nonexistent-codex-dock-test.sock'):
            with self.assertRaisesRegex(ValueError, 'unavailable'):
                setup.empty_node({'release': '/opt/test'})

    def test_existing_vhost_device_needs_no_module_load(self):
        with patch.object(Path, 'is_char_device', return_value=True), patch.object(setup, 'run') as command:
            setup.ensure_module_device('vhost_vsock', '/dev/vhost-vsock')
            command.assert_not_called()


if __name__ == '__main__':
    unittest.main()
