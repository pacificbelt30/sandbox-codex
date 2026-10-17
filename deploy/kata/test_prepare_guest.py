import pathlib
import struct
import tempfile
import unittest
from prepare_guest import geometry, static_hook


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
