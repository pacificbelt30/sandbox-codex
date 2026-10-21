"""The acceptance runner must never report a no-op/skip-only run as verified."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys
import unittest


SPEC = importlib.util.spec_from_file_location("verify_go_api", Path(__file__).with_name("verify-go-api.py"))
RUNNER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RUNNER)


class AcceptanceRunnerTests(unittest.TestCase):
    def run_events(self, events, status=0):
        payload = "\n".join(json.dumps(event) for event in events) + "\n"
        command = [sys.executable, "-c", "import sys; print(sys.argv[1]); sys.exit(int(sys.argv[2]))", payload, str(status)]
        with contextlib.redirect_stdout(io.StringIO()):
            return RUNNER.run_acceptance(command, Path(__file__).parent, None)

    def test_rejects_no_match_and_skips(self):
        for events in [[], [{"Action": "pass", "Test": "TestKataAPIIntegration"}],
                       [{"Action": "skip", "Test": "TestKataAPIIntegration/custom-profile"}]]:
            with self.subTest(events=events), self.assertRaisesRegex(RuntimeError, "no API acceptance subtests"):
                self.run_events(events)

    def test_requires_process_success_even_after_a_pass(self):
        with self.assertRaises(subprocess.CalledProcessError):
            self.run_events([{"Action": "pass", "Test": "TestKataAPIIntegration/compact"}], 1)

    def test_reports_executed_subtests(self):
        passed = self.run_events([{"Action": "pass", "Test": "TestKataAPIIntegration/compact"},
                                  {"Action": "pass", "Test": "TestKataAPIIntegration"}])
        self.assertEqual(passed, {"TestKataAPIIntegration/compact"})


if __name__ == "__main__":
    unittest.main()
