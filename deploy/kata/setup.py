#!/usr/bin/env python3
"""Transactional installer for a dedicated Ubuntu 26.04 amd64 Kata node.

Plan by default. Never edits the host Docker/default-containerd services.
An explicitly applied bundle is trusted operator code; its manifest checks
integrity and consistency, not publisher identity.
"""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import platform
import pwd
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time

HERE = Path(__file__).resolve().parent
LOCK = json.loads((HERE / 'assets.lock.json').read_text())
STATE = Path('/var/lib/codex-dock-setup')
SOCKET = '/run/codex-dock-containerd/containerd.sock'
SERVICES = ['codex-dock-node', 'codex-dock-buildkit', 'codex-dock-containerd']
MANAGED = [Path('/etc/systemd/system') / (s + '.service') for s in SERVICES] + [
    Path('/usr/local/bin/codex-dock'), Path('/usr/local/bin/buildctl'),
    Path('/etc/sysctl.d/80-codex-dock.conf'), Path('/etc/modules-load.d/codex-dock.conf')]
BUNDLE_FILES = ['codex-dock', 'codex-dock-worker-guard', 'codex-dock-io-logger']
PACKAGES = {'ip': 'iproute2', 'nft': 'nftables', 'sysctl': 'procps',
            'gcc': 'gcc', 'debugfs': 'e2fsprogs', 'zstd': 'zstd',
            'curl': 'curl', 'getent': 'libc-bin', 'usermod': 'passwd',
            'modprobe': 'kmod', 'tar': 'tar'}


def run(args, **kwargs):
    kwargs.setdefault('check', True)
    return subprocess.run([str(a) for a in args], text=True, **kwargs)


def sha(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def trusted(path, leaf=False):
    """Require root-owned, nonreplaceable installed paths; reject symlinks."""
    path = Path(path)
    for current in [path, *path.parents]:
        if not current.exists() and not current.is_symlink():
            continue
        info = current.lstat()
        if current.is_symlink() or info.st_uid != 0 or info.st_mode & 0o022:
            raise ValueError(f'untrusted install path: {current}')
    if leaf and not path.is_file():
        raise ValueError(f'missing installed file: {path}')


def atomic(path, data, mode=0o644):
    path = Path(path)
    trusted(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(dir=path.parent, delete=False) as stream:
        temporary = Path(stream.name)
        try:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
            os.fchmod(stream.fileno(), mode)
            os.replace(temporary, path)
        finally:
            temporary.unlink(missing_ok=True)
    fd = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def bundle_info(bundle):
    record = json.loads((bundle / 'bundle.json').read_text())
    if record.get('schema') != 1 or set(record.get('files', {})) != set(BUNDLE_FILES):
        raise ValueError('unsupported or incomplete node bundle')
    for name, checksum in record['files'].items():
        path = bundle / name
        if path.is_symlink() or not path.is_file() or sha(path) != checksum:
            raise ValueError(f'bundle checksum mismatch: {name}')
    return record


def host_check(operator):
    if platform.system() != 'Linux' or platform.machine() != 'x86_64':
        raise ValueError('automatic setup supports Linux x86_64 only')
    release = dict(line.split('=', 1) for line in Path('/etc/os-release').read_text().splitlines() if '=' in line)
    if release.get('ID', '').strip('"') != 'ubuntu' or release.get('VERSION_ID', '').strip('"') != '26.04':
        raise ValueError('automatic setup supports Ubuntu 26.04; use doc/kata.md for manual installation elsewhere')
    if not Path('/run/systemd/system').is_dir():
        raise ValueError('a running systemd host is required')
    if not Path('/dev/kvm').is_char_device():
        raise ValueError('KVM missing; enable CPU virtualization/nested virtualization and load the kvm module')
    controllers = Path('/sys/fs/cgroup/cgroup.controllers').read_text().split()
    if not {'cpu', 'memory', 'pids'}.issubset(controllers):
        raise ValueError('cgroup v2 with cpu, memory and pids is required')
    account = pwd.getpwnam(operator)
    if account.pw_uid == 0:
        raise ValueError('operator must be an existing non-root account')
    return account


def active(service):
    return subprocess.run(['systemctl', 'is-active', '--quiet', service]).returncode == 0


def ensure_module_device(module, device):
    if Path(device).is_char_device():
        return
    run(['modprobe', module])
    if not Path(device).is_char_device():
        raise ValueError(f'{module} did not provide {device}')


def empty_node(current):
    """Called after stopping admission/builds, before stopping containerd."""
    if Path(SOCKET).exists():
        ctr = Path(current['release']) / 'bin/ctr' if current else Path('/usr/bin/ctr')
        result = run([ctr, '--address', SOCKET, '--namespace', 'codex-dock', 'containers', 'list', '-q'], capture_output=True)
        if result.stdout.strip():
            raise ValueError('containers remain (including stopped workers/proxies); save work, remove workers, proxy stop/rm, then rerun setup')
        tasks = run([ctr, '--address', SOCKET, '--namespace', 'codex-dock', 'tasks', 'list', '-q'], capture_output=True)
        if tasks.stdout.strip():
            raise ValueError('native tasks remain; recover them with the existing node before updating')
    elif current:
        # An unavailable engine is not proof that no VMs exist.
        raise ValueError('dedicated containerd is unavailable; start the installed services and recover their state before updating')
    if list(Path('/var/lib/codex-dock-node/allocations').glob('*.json')):
        raise ValueError('VM allocations remain; inspect vm ps -a and recover exact owned resources first')
    group = Path('/sys/fs/cgroup/codex-dock')
    if group.exists() and 'populated 1' in (group / 'cgroup.events').read_text():
        raise ValueError('the dedicated VM cgroup is populated; refuse to update')


def fetch(name, cache, offline):
    asset = LOCK[name]
    target = cache / asset['file']
    if target.is_file() and not target.is_symlink() and sha(target) == asset['sha256']:
        return target
    if offline:
        raise ValueError(f'offline archive missing or checksum differs: {target}')
    cache.mkdir(parents=True, exist_ok=True)
    trusted(cache)
    temporary = cache / (asset['file'] + '.download')
    temporary.unlink(missing_ok=True)
    try:
        run(['curl', '--fail', '--location', '--proto', '=https', '--proto-redir', '=https', '--retry', '2', '--output', temporary, asset['url']])
        if sha(temporary) != asset['sha256']:
            raise ValueError(f'upstream checksum mismatch: {name}')
        temporary.chmod(0o644)
        temporary.replace(target)
    finally:
        temporary.unlink(missing_ok=True)
    return target


def extract_selected(archive, selected, zstd=False):
    """Stream only named regular files; never apply tar paths, owners or modes."""
    process = None
    stream = None
    found = set()
    try:
        if zstd:
            process = subprocess.Popen(['zstd', '-dc', str(archive)], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
            stream = process.stdout
            tar = tarfile.open(fileobj=stream, mode='r|')
        else:
            tar = tarfile.open(archive, mode='r|gz')
        with tar:
            for member in tar:
                name = member.name.removeprefix('./')
                if name not in selected:
                    continue
                if not member.isfile() or name in found:
                    raise ValueError('expected one regular archive payload: ' + name)
                destination, mode = selected[name]
                destination.parent.mkdir(parents=True, exist_ok=True)
                with tar.extractfile(member) as source, destination.open('xb') as output:
                    shutil.copyfileobj(source, output)
                destination.chmod(mode)
                found.add(name)
                if found == set(selected):
                    break
        if found != set(selected):
            raise ValueError('archive payload missing: ' + ', '.join(set(selected) - found))
    finally:
        if stream:
            stream.close()
        if process:
            if process.poll() is None:
                process.terminate()
            process.wait()


def stage(bundle, record, archives):
    releases = Path('/opt/codex-dock/releases')
    trusted(releases)
    releases.mkdir(parents=True, exist_ok=True)
    stamp = hashlib.sha256(json.dumps({'bundle': record, 'assets': LOCK,
        'installer': sha(HERE / 'setup.py'), 'configure': sha(HERE / 'configure.py')}, sort_keys=True).encode()).hexdigest()
    release = releases / (stamp[:16] + '-' + str(time.time_ns()))
    release.mkdir(mode=0o755)
    binary = release / 'bin'
    binary.mkdir()
    for name in BUNDLE_FILES:
        shutil.copyfile(bundle / name, binary / name)
        (binary / name).chmod(0o755)
        if sha(binary / name) != record['files'][name]:
            raise ValueError('bundle changed during copy')
    extract_selected(archives['containerd'], {'bin/' + name: (binary / name, 0o755)
        for name in ['containerd', 'ctr', 'containerd-shim-runc-v2']})
    extract_selected(archives['buildkit'], {'bin/' + name: (binary / name, 0o755)
        for name in ['buildctl', 'buildkitd']})
    shutil.copyfile(archives['runc'], binary / 'runc')
    (binary / 'runc').chmod(0o755)
    selected = {}
    for member, target, mode in [
        ('runtime-rs/bin/containerd-shim-kata-v2', 'containerd-shim-kata-v2', 0o755),
        ('share/kata-containers/vmlinux-6.18.35-202-dragonball-experimental', 'kernel', 0o644),
        ('share/kata-containers/kata-ubuntu-resolute.image', 'original.image', 0o644),
        ('share/defaults/kata-containers/runtime-rs/configuration-dragonball.toml', 'original.toml', 0o644)]:
        selected['opt/kata/' + member] = (release / target, mode)
    extract_selected(archives['kata'], selected, True)
    run(['gcc', '-static', '-Os', '-Wall', '-Wextra', '-Werror', HERE / 'pids_hook.c', '-o', release / 'pids-hook'])
    run([sys.executable, HERE / 'prepare_guest.py', 'build', '--source-image', release / 'original.image', '--hook-binary', release / 'pids-hook', '--output', release / 'guest'])
    run([sys.executable, HERE / 'prepare_guest.py', 'verify', release / 'guest/assets.lock.json'])
    from configure import configure
    config = configure((release / 'original.toml').read_text(), str(release / 'guest/guest.image'))
    config, count = re.subn(r'^kernel\s*=.*$', 'kernel = ' + json.dumps(str(release / 'kernel')), config, flags=re.MULTILINE)
    if count != 1:
        raise ValueError('expected exactly one kernel setting')
    (release / 'kata.toml').write_text(config)
    (release / 'containerd.toml').write_text('''version = 3
root = "/var/lib/codex-dock-containerd"
state = "/run/codex-dock-containerd"
disabled_plugins = ["io.containerd.cri.v1.images", "io.containerd.cri.v1.runtime"]
[grpc]
address = "/run/codex-dock-containerd/containerd.sock"
''')
    import grp
    gid = grp.getgrnam('codex-dock').gr_gid
    (release / 'buildkit.toml').write_text(f'''root = "/var/lib/codex-dock-buildkit"
[grpc]
address = ["unix:///run/codex-dock-buildkit/buildkitd.sock"]
uid = 0
gid = {gid}
[worker.oci]
enabled = false
[worker.containerd]
enabled = true
address = "{SOCKET}"
namespace = "codex-dock"
snapshotter = "overlayfs"
''')
    shutil.copyfile(HERE / 'prepare-node.sh', release / 'prepare-node.sh')
    (release / 'prepare-node.sh').chmod(0o755)
    env = dict(os.environ, PATH=str(binary) + ':/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin')
    run([binary / 'codex-dock', 'runtime-lock', '--kata-config', release / 'kata.toml', '--daemon-config', release / 'containerd.toml', '--containerd', binary / 'containerd', '--runtime-shim', release / 'containerd-shim-kata-v2', '--launcher', binary / 'codex-dock-worker-guard', '--output', release / 'kata-assets.json'], env=env)
    return {'schema': 1, 'release': str(release), 'stamp': stamp, 'bundle': record, 'assets': LOCK}


def units(release):
    base = Path(release)
    binary = base / 'bin'
    common = f'Environment="PATH={binary}:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"\n'
    daemon = f'''[Unit]
Description=Dedicated codex-dock containerd
After=network-online.target
[Service]
Type=notify
{common}ExecStart={binary}/containerd --config {base}/containerd.toml
Delegate=yes
KillMode=process
TasksMax=infinity
LimitNOFILE=1048576
TimeoutStartSec=0
TimeoutStopSec=120
Restart=on-failure
[Install]
WantedBy=multi-user.target
'''
    node = f'''[Unit]
Description=codex-dock Kata node
Requires=codex-dock-containerd.service
After=codex-dock-containerd.service
[Service]
{common}ExecStartPre={base}/prepare-node.sh
ExecStart={binary}/codex-dock node serve --manifest {base}/kata-assets.json
UMask=0077
Restart=on-failure
[Install]
WantedBy=multi-user.target
'''
    builder = f'''[Unit]
Description=codex-dock BuildKit
Requires=codex-dock-containerd.service
After=codex-dock-containerd.service
[Service]
{common}ExecStart={binary}/buildkitd --config {base}/buildkit.toml
Delegate=yes
KillMode=process
UMask=0007
Restart=on-failure
[Install]
WantedBy=multi-user.target
'''
    return dict(zip(MANAGED[:3], [node, builder, daemon]))


def backup_files(directory, paths):
    directory.mkdir(mode=0o700)
    files = []
    for i, path in enumerate(paths):
        trusted(path)
        if path.exists():
            saved = directory / str(i)
            shutil.copyfile(path, saved)
            saved.chmod(path.stat().st_mode & 0o777)
            files.append({'path': str(path), 'saved': str(saved), 'mode': saved.stat().st_mode & 0o777})
        else:
            files.append({'path': str(path), 'saved': None})
    return files


def restore_files(files):
    for item in files:
        path = Path(item['path'])
        if path not in MANAGED:
            raise ValueError('unexpected rollback destination')
        if item['saved']:
            trusted(item['saved'], True)
            atomic(path, Path(item['saved']).read_bytes(), item['mode'])
        else:
            trusted(path)
            path.unlink(missing_ok=True)


def start_and_check(release):
    run(['systemctl', 'daemon-reload'])
    run(['systemctl', 'start', *reversed(SERVICES)])
    binary = Path(release) / 'bin'
    env = dict(os.environ, PATH=str(binary) + ':' + os.environ.get('PATH', ''))
    # Service creation is asynchronous. The readiness check includes node attestation.
    deadline = time.monotonic() + 75
    while time.monotonic() < deadline:
        try:
            result = subprocess.run([str(binary / 'codex-dock'), 'runtime-check', '--json'], env=env, capture_output=True, text=True, timeout=20)
            checks = json.loads(result.stdout).get('checks', [])
            names = {c['name'] for c in checks if c['status'] == 'ok'}
            if result.returncode == 0 and {'node', 'builder', 'host', 'kvm', 'cgroups', 'forwarding', 'runtime'}.issubset(names) and all(active(s) for s in SERVICES):
                return
        except (subprocess.TimeoutExpired, json.JSONDecodeError):
            pass
        time.sleep(1)
    raise ValueError('new services failed readiness; inspect journalctl -u codex-dock-node -u codex-dock-buildkit; rolling back')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', type=Path)
    parser.add_argument('--operator', default='')
    parser.add_argument('--cache', type=Path, default=Path('/var/cache/codex-dock-setup'))
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--offline', action='store_true')
    parser.add_argument('--rollback', action='store_true')
    args = parser.parse_args()
    record = None if args.rollback else bundle_info(args.bundle.resolve())
    if not args.rollback:
        host_check(args.operator)
    print('Plan: rollback previous setup' if args.rollback else 'Plan: verified Ubuntu 26.04 amd64 node; Kata 4.2.0 / Dragonball / containerd 2.4.2 / BuildKit 0.34.0 / runc 1.4.3', flush=True)
    print('Stop admission/builds; require empty containers, tasks and VM allocations; keep images and projects. Install into a new root-owned release; switch only codex-dock services; verify readiness; restore previous files/services on failure.', flush=True)
    if not args.apply:
        print('No host changes. Add --apply as root. Operator access is root-equivalent. Runtime downloads are pinned in assets.lock.json.', flush=True)
        return
    if os.geteuid() != 0:
        raise ValueError('--apply requires root')
    os.umask(0o022)
    with open('/run/codex-dock-setup.lock', 'a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        apply(args, record)


def apply(args, record):
    trusted(STATE)
    STATE.mkdir(mode=0o755, parents=True, exist_ok=True)
    current_path = STATE / 'installed.json'
    trusted(current_path)
    current = json.loads(current_path.read_text()) if current_path.exists() else None
    for path in MANAGED:
        trusted(path)
        if not current and path.exists():
            raise ValueError(f'unmanaged installation exists: {path}; preserve it and use the manual migration guide')
    previously_active = [s for s in SERVICES if active(s)]
    stopped_admission = False
    transaction = None
    forward_before = Path('/proc/sys/net/ipv4/ip_forward').read_text().strip()
    enabled_before = [s for s in SERVICES if subprocess.run(['systemctl', 'is-enabled', '--quiet', s], stderr=subprocess.DEVNULL).returncode == 0]
    try:
        run(['systemctl', 'stop', *SERVICES[:2]]) if current else None
        stopped_admission = bool(current)
        empty_node(current)
        if args.rollback:
            if not current or not current.get('rollback'):
                raise ValueError('no successful previous transaction is available')
            rollback_path = Path(current['rollback'])
            trusted(rollback_path, True)
            previous_transaction = json.loads(rollback_path.read_text())
            backup = STATE / ('rollback-attempt-' + str(time.time_ns()))
            transaction = {'files': backup_files(backup, MANAGED), 'previous': current}
            run(['systemctl', 'stop', *SERVICES])
            run(['systemctl', 'disable', *SERVICES], check=False)
            restore_files(previous_transaction['files'])
            previous = previous_transaction['previous']
            if previous:
                start_and_check(previous['release'])
                for service in previous_transaction.get('enabled', previous_transaction['active']):
                    run(['systemctl', 'enable', service])
                atomic(current_path, json.dumps(previous, indent=2).encode())
            else:
                current_path.unlink(missing_ok=True)
                run(['systemctl', 'daemon-reload'])
            run(['sysctl', '-q', '-w', 'net.ipv4.ip_forward=' + previous_transaction.get('forward', forward_before)])
            print('Previous setup restored. Persistent images/projects were retained.', flush=True)
            return
        missing = sorted({pkg for tool, pkg in PACKAGES.items() if not shutil.which(tool)})
        if not Path('/usr/include/stdio.h').is_file():
            missing.append('libc6-dev')
        if missing and args.offline:
            raise ValueError('offline setup requires packages: ' + ', '.join(missing))
        if missing:
            run(['apt-get', 'update'])
            run(['apt-get', 'install', '-y', '--no-install-recommends', *missing])
        # CPU-specific modules are normally loaded by the host; vsock is shared transport.
        for module, device in [('vhost_vsock', '/dev/vhost-vsock'), ('vhost_net', '/dev/vhost-net')]:
            ensure_module_device(module, device)
        if subprocess.run(['getent', 'group', 'codex-dock'], stdout=subprocess.DEVNULL).returncode:
            run(['groupadd', '--system', 'codex-dock'])
        run(['usermod', '-aG', 'codex-dock', args.operator])
        cache = args.cache.resolve()
        trusted(cache)
        archives = {name: fetch(name, cache, args.offline) for name in LOCK}
        release = stage(args.bundle.resolve(), record, archives)
        backup = STATE / ('transaction-' + str(time.time_ns()))
        files = backup_files(backup, MANAGED)
        transaction = {'files': files, 'previous': current, 'active': previously_active, 'enabled': enabled_before, 'forward': forward_before}
        journal = backup / 'transaction.json'
        atomic(journal, json.dumps(transaction, indent=2).encode(), 0o600)
        run(['systemctl', 'stop', *SERVICES]) if current else None
        for path, text in units(release['release']).items():
            atomic(path, text.encode())
        binary = Path(release['release']) / 'bin'
        for name in ['codex-dock', 'buildctl']:
            atomic(Path('/usr/local/bin') / name, (binary / name).read_bytes(), 0o755)
        atomic(Path('/etc/sysctl.d/80-codex-dock.conf'), b'net.ipv4.ip_forward = 1\n')
        atomic(Path('/etc/modules-load.d/codex-dock.conf'), b'vhost_vsock\nvhost_net\n')
        run(['sysctl', '-q', '-w', 'net.ipv4.ip_forward=1'])
        start_and_check(release['release'])
        run(['systemctl', 'enable', *SERVICES])
        release['rollback'] = str(journal)
        atomic(current_path, json.dumps(release, indent=2).encode())
        print('Node ready. Log in again for codex-dock group membership, then run codex-dock init as the operator. Existing firewall policy is unchanged; forwarding must allow proxy egress.', flush=True)
    except Exception:
        if transaction:
            subprocess.run(['systemctl', 'stop', *SERVICES])
            subprocess.run(['systemctl', 'disable', *SERVICES], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            restore_files(transaction['files'])
            if current:
                atomic(current_path, json.dumps(current, indent=2).encode())
            else:
                current_path.unlink(missing_ok=True)
            run(['sysctl', '-q', '-w', 'net.ipv4.ip_forward=' + forward_before])
            run(['systemctl', 'daemon-reload'])
            for service in enabled_before:
                run(['systemctl', 'enable', service])
        if stopped_admission or transaction:
            if previously_active:
                run(['systemctl', 'start', *reversed(previously_active)])
        raise


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print(f'Setup failed: {error}', file=sys.stderr)
        sys.exit(1)
