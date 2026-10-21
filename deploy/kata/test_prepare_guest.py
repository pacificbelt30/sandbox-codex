import pathlib
import json
import struct
import tempfile
import unittest
from prepare_guest import geometry, static_hook, verify, sha, HOOK_PATH, SOURCE_SHA256


class GuestInputTests(unittest.TestCase):
    def test_truncated_image_is_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            image = pathlib.Path(temp)/"guest.image"
            mbr = bytearray(512)
            mbr[510:512] = b"\x55\xaa"
            mbr[454:458] = (6144).to_bytes(4, "little")
            mbr[458:462] = (512000).to_bytes(4, "little")
            image.write_bytes(mbr)
            with self.assertRaises(ValueError):
                geometry(image)

    def test_legacy_and_profile_guest_policy_metadata(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            names = ("guest.image", "pids-hook", "pids_hook.c")
            for name in names:
                (root / name).write_text("fixture-" + name)
                (root / name).chmod(0o644)
            record = {"schema_version": 1, "pids_limit": 512, "guest_hook_path": HOOK_PATH,
                      "source_image_sha256": SOURCE_SHA256,
                      "files": {name: sha(root / name) for name in names}}
            manifest = root / "assets.lock.json"
            manifest.write_text(json.dumps(record))
            self.assertEqual(verify(manifest)["schema_version"], 1)
            record.update(schema_version=2, pid_policy="oci-annotation-v1")
            manifest.write_text(json.dumps(record))
            self.assertEqual(verify(manifest)["schema_version"], 2)
            for version, policy in ((1, "oci-annotation-v1"), (2, ""), (3, "oci-annotation-v1")):
                record.update(schema_version=version, pid_policy=policy)
                manifest.write_text(json.dumps(record))
                with self.assertRaises(ValueError):
                    verify(manifest)

    def test_dynamic_hook_is_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            binary = pathlib.Path(temp)/"hook"
            header = bytearray(120)
            header[:6] = b"\x7fELF\x02\x01"
            struct.pack_into("<H", header, 18, 62)
            struct.pack_into("<Q", header, 32, 64)
            struct.pack_into("<HH", header, 54, 56, 1)
            struct.pack_into("<I", header, 64, 3)
            binary.write_bytes(header)
            with self.assertRaisesRegex(ValueError, "statically linked"):
                static_hook(binary)


if __name__ == "__main__":
    unittest.main()
