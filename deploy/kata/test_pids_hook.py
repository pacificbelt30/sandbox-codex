import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


@unittest.skipUnless(shutil.which("gcc"), "gcc required for the guest hook parser")
class PIDPolicyTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory(prefix="cxd-hook-parser-")
        cls.addClassCleanup(cls.temp.cleanup)
        root = Path(cls.temp.name)
        source = Path(__file__).with_name("pids_hook.c").resolve()
        wrapper = root / "parser.c"
        wrapper.write_text(
            '#define main hook_main\n#include ' + json.dumps(str(source)) +
            '\n#undef main\nint main(int argc,char **argv) {'
            'if(argc!=2)return 2;printf("%ld\\n",requested_limit(argv[1]));return 0;}\n'
        )
        cls.binary = root / "parser"
        subprocess.run(["gcc", "-Wall", "-Wextra", "-Werror", str(wrapper), "-o", str(cls.binary)], check=True)

    def parse(self, state, expected=None):
        if not isinstance(state, str):
            state = json.dumps(state)
        result = subprocess.run([str(self.binary), state], capture_output=True, text=True, check=False)
        if expected is None:
            self.assertNotEqual(result.returncode, 0)
        else:
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(int(result.stdout), expected)

    def test_legacy_and_exact_annotation(self):
        self.parse({}, 512)
        self.parse({"annotations": {}}, 512)
        for value in (64, 256, 512, 4096):
            self.parse({"annotations": {"io.codex-dock.pids-limit": str(value)}}, value)

    def test_annotation_values_cannot_masquerade_as_policy_keys(self):
        self.parse({"annotations": {"unrelated": "io.codex-dock.pids-limit", "io.codex-dock.pids-limit": "256"}}, 256)
        self.parse({"annotations": {"unrelated": '{"io.codex-dock.pids-limit":"4096"}'}}, 512)
        self.parse({"annotations": {"unrelated": "braces }{ and escaped quotes \\\"", "io.codex-dock.pids-limit": "256"}}, 256)

    def test_invalid_limits_and_duplicate_members_fail_closed(self):
        for value in ("", "0", "63", "4097", "10000000000000000000000", "-1", "256x", "2e2", " 256", "256 ", 256, None):
            self.parse({"annotations": {"io.codex-dock.pids-limit": value}})
        self.parse('{"annotations":{"io.codex-dock.pids-limit":"256","io.codex-dock.pids-limit":"4096"}}')
        self.parse('{"annotations":{},"annotations":{}}')
        self.parse('{"annotations":{"broken":"unterminated}')


if __name__ == "__main__":
    unittest.main()
