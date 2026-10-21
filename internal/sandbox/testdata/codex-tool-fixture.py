# Offline Responses fixture: the real Codex CLI must execute a shell tool.
import ctypes
import gzip
import http.server
import json
import os
import pathlib
import signal
import subprocess
import threading
import tempfile

# Match the actual PID 1 workload even though the host-admin exec API omits NNP.
assert ctypes.CDLL(None).prctl(38, 1, 0, 0, 0) == 0
marker = pathlib.Path('/workspace/codex-tool-executed')
marker.unlink(missing_ok=True)
class Fixture(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass
    def do_POST(self):
        body = self.rfile.read(int(self.headers['Content-Length']))
        if self.headers.get('Content-Encoding') == 'gzip':
            body = gzip.decompress(body)
        req = json.loads(body)
        print("FIXTURE_REQUEST", self.path, flush=True)
        outputs = [x for x in req.get('input', []) if x.get('type') in ('function_call_output', 'custom_tool_call_output')]
        if marker.exists():
            assert outputs, 'Codex did not return tool output'
            assert marker.read_text() == 'kata-tool-ok\n'
            item = {'type':'message','id':'msg_fixture','role':'assistant','status':'completed','content':[{'type':'output_text','text':'fixture complete','annotations':[]}]}
        else:
            tools = list(req.get('tools', []))
            for entry in req.get('input', []):
                if entry.get('type') == 'additional_tools':
                    tools.extend(entry.get('tools', []))
            for entry in list(tools):
                if entry.get('type') == 'namespace':
                    tools.extend(dict(t, namespace=entry['name']) for t in entry.get('tools', []))
            tool = next(x for x in tools if x.get('name') in ('exec', 'exec_command', 'shell_command', 'shell'))
            props = tool.get('parameters', {}).get('properties', {})
            command = 'printf "kata-tool-ok\\n" > /workspace/codex-tool-executed'
            args = {'command':command} if 'command' in props else {'cmd':command}
            if tool['name'] == 'shell':
                args['command'] = ['bash', '-c', command]
            item = {'type':'function_call','id':'fc_fixture','call_id':'call_fixture','name':tool['name'],'arguments':json.dumps(args),'status':'completed'}
            if tool['name'] == 'exec':
                item = {'type':'custom_tool_call','id':'ct_fixture','call_id':'call_fixture','name':'exec','namespace':tool['namespace'],'input':'text(await tools.exec_command('+json.dumps({'cmd':command,'yield_time_ms':1000})+'));'}
        response = {'id':'resp_fixture','object':'response','status':'completed','output':[item],'usage':{'input_tokens':1,'output_tokens':1,'total_tokens':2}}
        events = [{'type':'response.created','response':dict(response,status='in_progress',output=[])}, {'type':'response.output_item.added','output_index':0,'item':item}, {'type':'response.output_item.done','output_index':0,'item':item}, {'type':'response.completed','response':response}]
        self.send_response(200)
        self.send_header('Content-Type','text/event-stream')
        self.end_headers()
        for event in events:
            self.wfile.write(('event: '+event['type']+'\ndata: '+json.dumps(event)+'\n\n').encode())
        self.wfile.flush()
server = http.server.ThreadingHTTPServer(('127.0.0.1',0),Fixture)
threading.Thread(target=server.serve_forever,daemon=True).start()
# Keep the product-generated sandbox setting. Only the API destination is
# replaced. No sandbox override or dangerous approval bypass is passed.
# Codex exec selects its own noninteractive approval policy.
config = pathlib.Path(os.environ['HOME']) / '.codex/config.toml'
assert 'sandbox_mode = "danger-full-access"' in config.read_text()
environment = dict(os.environ, OPENAI_API_KEY="sk-local-fixture-never-valid", NO_PROXY="127.0.0.1,localhost")
for key in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
    environment.pop(key, None)
log = tempfile.TemporaryFile(mode="w+t")
# Keep this offline test independent of optional external metrics delivery.
# https://learn.chatgpt.com/docs/config-file/config-reference
command = [
    'codex', '--ask-for-approval', 'on-request', 'exec', '--skip-git-repo-check',
    '-c', 'approval_policy="on-request"',
    '-c', 'analytics.enabled=false', '-c', 'otel.metrics_exporter="none"',
    '-c', f'openai_base_url="http://127.0.0.1:{server.server_port}/v1"',
    '-c', 'features.responses_websockets=false',
    '-c', 'features.responses_websockets_v2=false',
    '-c', 'model_provider="fixture"',
    '-c', f'model_providers.fixture={{name="fixture",base_url="http://127.0.0.1:{server.server_port}/v1",wire_api="responses"}}',
    'Use the shell tool to create the fixture marker.',
]
process = subprocess.Popen(command, env=environment, stdin=subprocess.DEVNULL, text=True, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
try:
    process.wait(timeout=45)
except subprocess.TimeoutExpired:
    os.killpg(process.pid, signal.SIGKILL)
    process.wait()
    log.seek(0)
    print(log.read(), flush=True)
    raise
log.seek(0)
print(log.read(), flush=True)
log.close()
assert process.returncode == 0, process.returncode
assert marker.read_text() == 'kata-tool-ok\n'
server.shutdown()
print('REAL_CODEX_TOOL_EXECUTION_OK')
