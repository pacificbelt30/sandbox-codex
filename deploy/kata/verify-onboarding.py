#!/usr/bin/env python3
"""Disposable-node first use: empty HOME, keyring login, init and real Kata.

Only invalid credentials are used; --skip-check prevents provider API calls.
The first init builds the default worker image from upstream package sources.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import tomllib

CLI = '/usr/local/bin/oaiso'
MASTER = b'onboarding-fixture-keyring-password-never-valid'


def run(args, label, evidence, secret, timeout=900, **kwargs):
    result = subprocess.run([CLI, *args], capture_output=True, timeout=timeout, **kwargs)
    output = result.stdout + result.stderr
    if secret.encode() in output or MASTER in output:
        raise RuntimeError('credential leaked in ' + label)
    (evidence / (label + '.txt')).write_bytes(output)
    if result.returncode:
        raise RuntimeError(label + ' failed: ' + output[-4000:].decode(errors='replace'))
    return result.stdout


def inside(agent):
    evidence = Path(os.environ['OAISO_ONBOARDING_EVIDENCE'])
    secret = 'fixture-onboarding-' + agent + '-never-valid'
    project = Path.home() / 'project'
    project.mkdir(mode=0o700)
    run(['runtime-check'], agent + '-runtime', evidence, secret)
    run(['auth', 'keyring', 'check'], agent + '-keyring', evidence, secret)
    run(['login', '--agent', agent, '--method', 'api-key', '--profile', 'work',
         '--key-stdin', '--skip-check'], agent + '-login', evidence, secret,
        input=secret.encode())
    config_path = Path.home() / '.config/codex-dock/config.toml'
    config = tomllib.loads(config_path.read_text())
    assert config['auth_profile'] == 'work', 'login did not select its profile'
    assert config_path.stat().st_mode & 0o077 == 0, 'unsafe configuration permissions'
    assert Path(config['proxy']['admin_secret_file']).stat().st_mode & 0o077 == 0
    vault = Path.home() / '.config/codex-dock/profiles/work/work.vault'
    record = json.loads(vault.read_text())
    assert record['header']['backend'] == 'secret-service'
    assert not vault.with_suffix('.json').exists(), 'plaintext fallback'
    assert secret not in vault.read_text(), 'API key stored in plaintext'
    assert 'default_image' not in config, 'test silently changed the default image'
    run(['init', '--agent', agent], agent + '-init', evidence, secret, cwd=project)
    output = run([agent, '--version'], agent + '-agent-version', evidence, secret, cwd=project, timeout=120)
    expected = 'codex-cli 0.163.0' if agent == 'codex' else '2.1.306'
    assert expected in output.decode(), 'fixed agent did not start in the VM'
    terminal = 'first-use-terminal-' + agent
    try:
        program = "python3 -c \"print('terminal-input-ready',flush=True);assert input()=='fixture-input';print('terminal-input-ok',flush=True)\""
        output = run(['run', '--name', terminal, '--keep', '--no-internet', '--exec', program],
                     agent + '-terminal', evidence, secret, cwd=project,
                     input=b'fixture-input\n', timeout=120)
        assert b'terminal-input-ok' in output, 'primary TTY input/output failed'
        output = run(['logs', terminal], agent + '-terminal-logs', evidence, secret)
        assert b'terminal-input-ok' in output, 'terminal history missing'
    finally:
        run(['rm', '--force', terminal], agent + '-terminal-remove', evidence, secret)
    name = 'first-use-' + agent
    try:
        # Match the documented run flow, and prove the selected profile and
        # numeric ownership work without a test-specific config or image flag.
        program = "python3 -c \"import time;from pathlib import Path;Path('/workspace/first-use-marker').write_text('ok');time.sleep(300)\""
        run(['run', '--name', name, '--detach', '--keep', '--no-internet', '--exec', program],
            agent + '-worker', evidence, secret, cwd=project)
        for _ in range(150):
            if (project / 'first-use-marker').exists():
                break
            time.sleep(.2)
        assert (project / 'first-use-marker').read_text() == 'ok'
        assert (project / 'first-use-marker').stat().st_uid == os.getuid()
        run(['vm', 'inspect', name], agent + '-inspect', evidence, secret)
    finally:
        run(['rm', '--force', name], agent + '-remove', evidence, secret)
        run(['auth', 'profile', 'lock', 'work'], agent + '-lock', evidence, secret)
        run(['proxy', 'stop'], agent + '-proxy-stop', evidence, secret)
        run(['proxy', 'rm'], agent + '-proxy-remove', evidence, secret)
        run(['network', 'rm'], agent + '-network-remove', evidence, secret)
    print('PASS: ' + agent + ' fresh login -> selected keyring profile -> default init -> fixed CLI -> Kata workspace')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--inside', choices=['codex', 'claude'])
    parser.add_argument('--home-base', type=Path)
    parser.add_argument('--evidence', type=Path)
    args = parser.parse_args()
    if args.inside:
        inside(args.inside)
        return
    if os.environ.get('CODEX_DOCK_DISPOSABLE_NODE') != '1' or os.geteuid() == 0:
        raise SystemExit('requires a normal user in an explicitly disposable node')
    for agent in ['codex', 'claude']:
        home = args.home_base / agent
        home.mkdir(mode=0o700, parents=True)
        env = os.environ | {'HOME': str(home), 'OAISO_ONBOARDING_EVIDENCE': str(args.evidence),
                            'OPENAI_API_KEY': 'host-key-must-not-be-imported',
                            'ANTHROPIC_API_KEY': 'host-key-must-not-be-imported'}
        # The private keyring has no dependency on a desktop/user service.
        env.pop('DBUS_SESSION_BUS_ADDRESS', None)
        input_fd, writer = os.pipe()
        os.write(writer, MASTER)
        os.close(writer)
        try:
            result = subprocess.run([CLI, '--passphrase-fd', str(input_fd), 'auth', 'keyring',
                                     'session', '--', sys.executable, __file__, '--inside', agent],
                                    env=env, pass_fds=(input_fd,), capture_output=True, timeout=1200)
        finally:
            os.close(input_fd)
        output = result.stdout + result.stderr
        if MASTER in output or ('fixture-onboarding-' + agent + '-never-valid').encode() in output:
            raise RuntimeError('onboarding session exposed a credential')
        (args.evidence / (agent + '-session.txt')).write_bytes(output)
        if result.returncode:
            raise RuntimeError(output[-6000:].decode(errors='replace'))
        print(output.decode().strip())


if __name__ == '__main__':
    main()
