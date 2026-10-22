#!/usr/bin/env python3
"""Record the integrity of a matching, locally built Linux amd64 node bundle."""
import hashlib
import json
from pathlib import Path
import sys

def main():
    directory = Path(sys.argv[1])
    files = {}
    for name in ['oaiso', 'codex-dock-worker-guard', 'codex-dock-io-logger']:
        path = directory / name
        with path.open('rb') as stream:
            header = stream.read(20)
            if header[:6] != b'\x7fELF\x02\x01' or header[18:20] != b'\x3e\x00':
                raise ValueError('bundle binaries must be Linux amd64 ELF')
            stream.seek(0)
            files[name] = hashlib.file_digest(stream, 'sha256').hexdigest()
    (directory / 'bundle.json').write_text(json.dumps({'schema': 1, 'version': sys.argv[2], 'files': files}, indent=2) + '\n')

if __name__ == '__main__':
    main()
