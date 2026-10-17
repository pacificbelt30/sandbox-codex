# Offline filesystem probe, launched through the worker's real entrypoint.
import json
import os
from pathlib import Path
import time
import traceback

assert os.environ.get('CODEX_DOCK_VERIFIED_KATA') == '1', 'probe requires the Kata bootstrap gate'
control = Path('/control')
(control / 'ready.json').write_text(json.dumps({
    'uid': os.getuid(), 'gid': os.getgid(), 'groups': os.getgroups(),
    'cwd': os.getcwd(),
}))
sequence = 1
while True:
    request = control / f'request-{sequence}.json'
    if not request.exists():
        time.sleep(0.02)
        continue
    try:
        code = json.loads(request.read_text())['code']
        scope = {'os': os, 'Path': Path, 'time': time}
        exec(code, scope)
        response = {'value': scope['result']}
    except Exception:
        response = {'error': traceback.format_exc()}
    temporary = control / f'response-{sequence}.tmp'
    temporary.write_text(json.dumps(response))
    temporary.replace(control / f'response-{sequence}.json')
    sequence += 1
