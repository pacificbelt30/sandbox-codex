import os
from pathlib import Path
import subprocess
import sys
import shutil
import tempfile
import unittest


SCRIPT = Path(__file__).with_name('install.sh')


class InstallScriptTests(unittest.TestCase):
    def fake_uv(self, directory):
        uv = directory / 'uv'
        uv.write_text('#!/bin/sh\n'
                      'if [ -n "$INSTALL_TEST_UV_LOG" ]; then printf "%s\\n" "$@" > "$INSTALL_TEST_UV_LOG"; fi\n'
                      '[ "${INSTALL_TEST_UV_FAIL:-0}" = 0 ] || exit 9\n'
                      'printf "%s\\n" "$INSTALL_TEST_PYTHON"\n')
        uv.chmod(0o755)
        return {'INSTALL_TEST_PYTHON': sys.executable}

    def test_plan_precedes_apply_and_forwards_options(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            bundle = root / 'bundle with spaces'
            bundle.mkdir()
            (bundle / 'bundle.json').write_text('{}')
            log = root / 'calls'
            cli = bundle / 'oaiso'
            cli.write_text('#!/bin/sh\nprintf "cli:%s\\n" "$*" >> "$INSTALL_TEST_LOG"\n')
            cli.chmod(0o755)
            fake_bin = root / 'bin'
            fake_bin.mkdir()
            uv_env = self.fake_uv(fake_bin)
            sudo = fake_bin / 'sudo'
            sudo.write_text('#!/bin/sh\nprintf "sudo\\n" >> "$INSTALL_TEST_LOG"\n[ "$1" = -- ] && shift\nexec "$@"\n')
            sudo.chmod(0o755)
            env = os.environ | uv_env | {'PATH': str(fake_bin) + os.pathsep + os.environ['PATH'], 'INSTALL_TEST_LOG': str(log)}
            args = [str(SCRIPT), '--bundle', str(bundle), '--operator', 'alice', '--cache', str(root / 'cache'), '--offline']

            subprocess.run(args, env=env, check=True, capture_output=True, text=True)
            calls = log.read_text().splitlines()
            self.assertEqual(len(calls), 1)
            self.assertIn('--bundle ' + str(bundle), calls[0])
            self.assertIn('--operator alice', calls[0])
            self.assertIn('--python ' + sys.executable, calls[0])
            self.assertIn('--offline', calls[0])
            self.assertNotIn('--apply', calls[0])

            log.unlink()
            subprocess.run(args + ['--apply'], env=env, check=True, capture_output=True, text=True)
            calls = log.read_text().splitlines()
            self.assertEqual(calls[0], calls[2].removesuffix(' --apply'))
            self.assertEqual(calls[1], 'sudo')
            self.assertTrue(calls[2].endswith(' --apply'))

    def test_failed_plan_never_attempts_apply(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            bundle = root / 'bundle'
            bundle.mkdir()
            (bundle / 'bundle.json').write_text('{}')
            cli = bundle / 'oaiso'
            cli.write_text('#!/bin/sh\nexit 7\n')
            cli.chmod(0o755)
            fake_bin = root / 'bin'
            fake_bin.mkdir()
            uv_env = self.fake_uv(fake_bin)
            sudo = fake_bin / 'sudo'
            sudo.write_text('#!/bin/sh\ntouch "$INSTALL_TEST_SUDO_CALLED"\n')
            sudo.chmod(0o755)
            marker = root / 'sudo-called'
            env = os.environ | uv_env | {'PATH': str(fake_bin) + os.pathsep + os.environ['PATH'], 'INSTALL_TEST_SUDO_CALLED': str(marker)}
            result = subprocess.run([str(SCRIPT), '--bundle', str(bundle), '--operator', 'alice', '--apply'], env=env, capture_output=True)
            self.assertEqual(result.returncode, 7)
            self.assertFalse(marker.exists())

    def test_offline_python_failure_prevents_plan_and_sudo(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            uv_env = self.fake_uv(root)
            log = root / 'uv-log'
            env = os.environ | uv_env | {'PATH': str(root) + os.pathsep + os.environ['PATH'],
                                        'INSTALL_TEST_UV_FAIL': '1', 'INSTALL_TEST_UV_LOG': str(log)}
            result = subprocess.run([str(SCRIPT), '--operator', 'alice', '--offline', '--apply'],
                                    env=env, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('cannot prepare uv-managed Python', result.stderr)
            args = log.read_text().splitlines()
            self.assertIn('--offline', args)
            self.assertIn('--no-python-downloads', args)

    def test_missing_uv_is_actionable_even_when_python_is_installed(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for name in ['bash', 'dirname', 'python3']:
                (root / name).symlink_to(shutil.which(name))
            result = subprocess.run([str(SCRIPT), '--operator', 'alice'],
                                    env=os.environ | {'PATH': str(root)}, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('uv is required', result.stderr)

    def test_root_requires_operator_before_python_download(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            uv_env = self.fake_uv(root)
            fake_id = root / 'id'
            fake_id.write_text('#!/bin/sh\nprintf "0\\n"\n')
            fake_id.chmod(0o755)
            log = root / 'uv-log'
            result = subprocess.run([str(SCRIPT)], capture_output=True, text=True,
                                    env=os.environ | uv_env | {'PATH': str(root) + os.pathsep + os.environ['PATH'],
                                                              'INSTALL_TEST_UV_LOG': str(log)})
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('pass --operator USER', result.stderr)
            self.assertFalse(log.exists())


if __name__ == '__main__':
    unittest.main()
