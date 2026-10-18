#!/usr/bin/env python3
"""Run setup/update/recovery acceptance in a disposable private Ubuntu/KVM node.

Requires Docker ONLY as this test's outer disposable host. The installed product
inside it uses native containerd/Kata/Dragonball and contains no Docker daemon.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', type=Path, required=True)
    parser.add_argument('--next-bundle', type=Path, required=True)
    parser.add_argument('--images', type=Path, required=True)
    parser.add_argument('--acceptance', type=Path, required=True)
    parser.add_argument('--cache', type=Path, default=ROOT / '.kata-verification/setup-e2e/cache')
    parser.add_argument('--proxy-image', default='codex-dock-proxy:policy-verification')
    parser.add_argument('--image', default='codex-dock-setup-node:verification')
    parser.add_argument('--sudo-docker', action='store_true')
    parser.add_argument('--offline', action='store_true')
    args = parser.parse_args()
    if os.environ.get('CODEX_DOCK_DISPOSABLE_SETUP') != '1':
        raise SystemExit('set CODEX_DOCK_DISPOSABLE_SETUP=1 on a dedicated Linux/KVM test host')
    if not Path('/dev/kvm').exists():
        raise SystemExit('/dev/kvm is required; this test never skips missing prerequisites')
    env = dict(os.environ)
    for key in ['DOCKER_HOST', 'DOCKER_CONTEXT', 'DOCKER_TLS', 'DOCKER_TLS_VERIFY', 'DOCKER_CERT_PATH']:
        env.pop(key, None)
    docker = (['sudo', '-n'] if args.sudo_docker else []) + ['docker', '--host=unix:///var/run/docker.sock']
    def call(argv, **kwargs):
        return subprocess.run(docker + [str(x) for x in argv], env=env, check=True, **kwargs)
    call(['info'], stdout=subprocess.DEVNULL)
    assets = json.loads((ROOT / 'deploy/kata/assets.lock.json').read_text())
    args.cache.mkdir(parents=True, exist_ok=True)
    for name, asset in assets.items():
        path = args.cache / asset['file']
        if not path.is_file():
            if args.offline:
                raise SystemExit('missing offline asset: ' + str(path))
            subprocess.run(['curl', '--fail', '--location', '--proto', '=https', '--proto-redir', '=https', '--retry', '2', '--output', str(path), asset['url']], check=True)
        with path.open('rb') as stream:
            if hashlib.file_digest(stream, 'sha256').hexdigest() != asset['sha256']:
                raise SystemExit('asset checksum mismatch: ' + name)
    identity = uuid.uuid4().hex[:12]
    name = 'codex-dock-setup-' + identity
    volume = name + '-data'
    evidence = ROOT / '.kata-verification/setup-e2e' / identity
    evidence.mkdir(parents=True)
    started = False
    volume_created = False
    try:
        call(['volume', 'create', '--label', 'codex-dock.setup-test=' + identity, volume], stdout=subprocess.DEVNULL)
        volume_created = True
        argv = ['run', '-d', '--name', name, '--label', 'codex-dock.setup-test=' + identity,
                '--privileged', '--cgroupns=private', '--network=none',
                '--device', '/dev/kvm', '--device', '/dev/vhost-vsock', '--device', '/dev/vhost-net',
                '-e', 'CODEX_DOCK_DISPOSABLE_NODE=1', '-e', 'CODEX_DOCK_KATA_PROXY_IMAGE='+args.proxy_image,
                '--mount', f'type=volume,src={volume},dst=/var/lib/codex-dock-containerd']
        for source, target, readonly in [
            (args.bundle, '/bundle', True), (args.next_bundle, '/bundle-next', True),
            (args.images, '/images.tar', True), (args.acceptance, '/acceptance.test', True),
            (args.cache, '/cache-input', True), (ROOT / 'deploy/kata', '/verification', True),
            (evidence, '/evidence', False)]:
            argv += ['--mount', f'type=bind,src={source.resolve()},dst={target}' + (',readonly' if readonly else '')]
        argv += [args.image, '/bin/sh', '/verification/verify-boot.sh']
        call(argv, stdout=subprocess.DEVNULL)
        started = True
        for _ in range(100):
            result = subprocess.run(docker + ['exec', name, 'test', '-S', '/run/systemd/private'], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            if result.returncode == 0:
                break
            time.sleep(.2)
        else:
            raise RuntimeError('disposable systemd did not start')
        with (evidence / 'controller.log').open('w') as log:
            call(['exec', name, '/bin/bash', '/verification/verify-node.sh'], stdout=log, stderr=subprocess.STDOUT, timeout=2400)
        print('PASS: fresh setup, real Kata, upgrade and recovery. Evidence:', evidence)
    finally:
        if started:
            # Only this controller's uniquely labelled node may be removed.
            label = subprocess.check_output(docker + ['inspect', '--format', '{{index .Config.Labels "codex-dock.setup-test"}}', name], env=env, text=True).strip()
            if label != identity:
                raise RuntimeError('test node identity changed; refusing cleanup')
            with (evidence / 'journal.log').open('w') as log:
                subprocess.run(docker + ['exec', name, 'journalctl', '--no-pager', '-u', 'codex-dock-node', '-u', 'codex-dock-buildkit', '-u', 'codex-dock-containerd', '-n', '250'], env=env, stdout=log, stderr=subprocess.STDOUT)
            subprocess.run(docker + ['exec', name, 'chown', '-R', f'{os.getuid()}:{os.getgid()}', '/evidence'], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            call(['stop', '--time', '45', name], stdout=subprocess.DEVNULL)
            call(['rm', name], stdout=subprocess.DEVNULL)
        if volume_created:
            label = subprocess.check_output(docker + ['volume', 'inspect', '--format', '{{index .Labels "codex-dock.setup-test"}}', volume], env=env, text=True).strip()
            if label != identity:
                raise RuntimeError('test volume identity changed; refusing cleanup')
            call(['volume', 'rm', volume], stdout=subprocess.DEVNULL)


if __name__ == '__main__':
    main()
