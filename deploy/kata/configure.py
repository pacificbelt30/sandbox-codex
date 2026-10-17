#!/usr/bin/env python3
"""Derive a worker profile from a trusted Kata Rust/Dragonball config.

This does not install or download Kata. Verify the release bundle first.
"""
import argparse
import hashlib
import json
import pathlib
import re
import tomllib


def configure(source: str, image: str) -> str:
    parsed = tomllib.loads(source)
    hypervisor = parsed.get("hypervisor", {}).get("dragonball", {})
    if hypervisor.get("shared_fs") != "inline-virtio-fs":
        raise ValueError("expected a Dragonball config with inline-virtio-fs")
    if not pathlib.PurePosixPath(image).is_absolute():
        raise ValueError("guest image must be an absolute path")
    for key, value in {
        "image": json.dumps(image),
        # The host and guest edit the same workspace. Close-to-open caching
        # can retain old host contents even after a client reopens a file.
        "virtio_fs_cache": json.dumps("none"),
        # Dragonball defaults to writeback/no_open independently of cache mode.
        # Keep real opens and direct reads so live host edits are coherent.
        "virtio_fs_extra_args": json.dumps([
            "--thread-pool-size=1", "-o", "open,no_writeback_cache"
        ]),
        "enable_annotations": "[]",
        "disable_guest_seccomp": "false",
        "guest_hook_path": json.dumps("/usr/share/codex-dock/hooks"),
        "sandbox_cgroup_only": "true",
        "default_memory": "2560",
        "overhead_memory": "512",
        "overhead_vcpus": "0.0",
        "static_sandbox_resource_mgmt": "true",
        "hypervisor_name": json.dumps("dragonball"),
        "default_maxmemory": "4096",
    }.items():
        source, count = re.subn(rf"^{key}\s*=.*$", lambda _: f"{key} = {value}", source, flags=re.MULTILINE)
        if count != 1:
            raise ValueError(f"expected exactly one {key} setting")
    result = tomllib.loads(source)
    if result["runtime"]["disable_guest_seccomp"]:
        raise ValueError("guest seccomp must stay enabled")
    return source


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=pathlib.Path, required=True)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--image", default="/opt/codex-dock-kata/guest.image")
    args = parser.parse_args()
    original = args.source.read_text()
    result = configure(original, args.image)
    # Exclusive creation prevents overwriting the source or an active profile.
    with args.output.open("x") as output:
        output.write(result)
    print(json.dumps({"source_sha256": hashlib.sha256(original.encode()).hexdigest(),
                      "profile_sha256": hashlib.sha256(result.encode()).hexdigest(),
                      "output": str(args.output)}, sort_keys=True))


if __name__ == "__main__":
    main()
