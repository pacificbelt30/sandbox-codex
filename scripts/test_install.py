import os
from pathlib import Path
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).with_name('install.sh')


class InstallScriptTests(unittest.TestCase):
    def test_plan_precedes_apply_and_forwards_options(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            bundle = root / 'bundle with spaces'
            bundle.mkdir()
            (bundle / 'bundle.json').write_text('{}')
            log = root / 'calls'
            cli = bundle / 'codex-dock'
            cli.write_text('#!/bin/sh\nprintf "cli:%s\\n" "$*" >> "$INSTALL_TEST_LOG"\n')
            cli.chmod(0o755)
            fake_bin = root / 'bin'
            fake_bin.mkdir()
            sudo = fake_bin / 'sudo'
            sudo.write_text('#!/bin/sh\nprintf "sudo\\n" >> "$INSTALL_TEST_LOG"\n[ "$1" = -- ] && shift\nexec "$@"\n')
            sudo.chmod(0o755)
            env = os.environ | {'PATH': str(fake_bin) + os.pathsep + os.environ['PATH'], 'INSTALL_TEST_LOG': str(log)}
            args = [str(SCRIPT), '--bundle', str(bundle), '--operator', 'alice', '--cache', str(root / 'cache'), '--offline']

            subprocess.run(args, env=env, check=True, capture_output=True, text=True)
            calls = log.read_text().splitlines()
            self.assertEqual(len(calls), 1)
            self.assertIn('--bundle ' + str(bundle), calls[0])
            self.assertIn('--operator alice', calls[0])
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
            cli = bundle / 'codex-dock'
            cli.write_text('#!/bin/sh\nexit 7\n')
            cli.chmod(0o755)
            fake_bin = root / 'bin'
            fake_bin.mkdir()
            sudo = fake_bin / 'sudo'
            sudo.write_text('#!/bin/sh\ntouch "$INSTALL_TEST_SUDO_CALLED"\n')
            sudo.chmod(0o755)
            marker = root / 'sudo-called'
            env = os.environ | {'PATH': str(fake_bin) + os.pathsep + os.environ['PATH'], 'INSTALL_TEST_SUDO_CALLED': str(marker)}
            result = subprocess.run([str(SCRIPT), '--bundle', str(bundle), '--operator', 'alice', '--apply'], env=env, capture_output=True)
            self.assertEqual(result.returncode, 7)
            self.assertFalse(marker.exists())


if __name__ == '__main__':
    unittest.main()
