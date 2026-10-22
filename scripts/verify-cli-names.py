#!/usr/bin/env python3
"""Verify the sole Oaiso entry point without a node or external services."""
import argparse
import os
from pathlib import Path
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True)
    args = parser.parse_args()
    binary = Path(args.binary).resolve()
    with tempfile.TemporaryDirectory(prefix='oaiso-cli-') as temporary:
        directory = Path(temporary)
        home = directory / 'home'
        home.mkdir()
        env = {'HOME': str(home), 'PATH': os.environ.get('PATH', '/usr/bin:/bin')}
        secret = 'fixture-oaiso-private-key'

        def run(command, *arguments, input=None):
            result = subprocess.run([str(command), *arguments], env=env, input=input,
                                    text=True, capture_output=True, timeout=10, check=False)
            assert result.returncode == 0, result.stderr
            assert secret not in result.stdout + result.stderr, 'credential leaked to CLI output'
            return result.stdout

        names = ['oaiso']
        for name in names:
            (directory / name).symlink_to(binary)
        run(directory / 'oaiso', 'auth', 'profile', 'set', 'work', '--key-stdin', input=secret)
        config = home / '.config/codex-dock/config.toml'
        config.write_text('auth_profile = "work"\n')
        for name in names:
            cli = directory / name
            help_text = run(cli, '--help')
            assert '御AgentISOlation (Oaiso)' in help_text and f'  {name} [command]' in help_text
            assert run(cli, '--version').startswith(name + ' version ')
            for subcommand in [('run',), ('auth', 'profile'), ('update',)]:
                text = run(cli, *subcommand, '--help')
                assert 'oaiso' in text or 'Oaiso' in text or name in text
            for shell in ['bash', 'zsh', 'fish', 'powershell']:
                completion = run(cli, 'completion', shell)
                marker = {'bash': f'__start_{name} {name}', 'zsh': f'#compdef {name}',
                          'fish': f'complete -c {name}',
                          'powershell': f"-CommandName '{name}'"}[shell]
                assert marker in completion, f'{shell} completion lost {name}'
            assert 'codex' in run(cli, '__complete', 'default', '')
            assert 'Profile: work' in run(cli, 'auth', 'show'), 'legacy profile configuration was not reused'
        profile = home / '.config/codex-dock/profiles/work/work.json'
        assert profile.is_file() and profile.stat().st_mode & 0o777 == 0o600
    print('PASS: oaiso help, version, completion, commands and existing auth-profile storage')


if __name__ == '__main__':
    main()
