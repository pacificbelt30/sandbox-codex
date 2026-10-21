#!/usr/bin/env python3
"""Run public Go API acceptance on a prepared, explicitly selected Kata node.

Builds a test-only auth proxy with synthetic upstreams; never deploy that image.
The caller supplies the node, base proxy/worker images and standalone BuildKit.
"""
import argparse
import os
from pathlib import Path
import re
import subprocess
import tempfile
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--run", default="^TestKataAPIIntegration$", help="Go test name/subtest expression")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parent.parent
    binary = str(Path(args.binary).resolve())
    base = os.environ.get("CODEX_DOCK_KATA_PROXY_IMAGE", "codex-dock-proxy:latest")
    if not re.fullmatch(r"[A-Za-z0-9_./:@-]+", base):
        parser.error("invalid base proxy image reference")
    if not re.fullmatch(r"[A-Za-z0-9_.+-]+", args.version):
        parser.error("invalid build version")
    version_flag = "-X github.com/pacificbelt30/codex-dock/internal/buildinfo.Version=" + args.version
    image = "codex-dock-proxy:api-fixture-" + uuid.uuid4().hex[:12]
    env = dict(os.environ, CGO_ENABLED="0", GOOS="linux", GOARCH="amd64")
    with tempfile.TemporaryDirectory(prefix="cxd-api-fixture-") as directory:
        work = Path(directory)
        subprocess.run(["go", "test", "-c", "-ldflags", version_flag, "./internal/authproxy", "-o", str(work / "fixture.test")], cwd=repo, env=env, check=True)
        (work / "Dockerfile").write_text(
            f"FROM {base}\nCOPY --chmod=0755 fixture.test /codex-dock\n"
            'ENV CXD_KATA_FIXTURE_PROXY=1\nLABEL io.codex-dock.proxy-protocol="external-store-v1"\n'
            f'LABEL io.codex-dock.proxy-build="{args.version}"\n'
        )
        subprocess.run([binary, "build", "-t", image, "-f", "Dockerfile"], cwd=work, check=True)
        subprocess.run(["go", "test", "-ldflags", version_flag, "./pkg/cxd", "-run", args.run, "-count=1", "-v", "-timeout=20m"], cwd=repo,
                       env=dict(os.environ, CODEX_DOCK_KATA_API_E2E="1", CODEX_DOCK_KATA_API_PROXY_IMAGE=image, CODEX_DOCK_KATA_API_OWN_IMAGE="1"), check=True)


if __name__ == "__main__":
    main()
