#!/usr/bin/env python3
"""Measure cached OCI builds and Docker/Kata readiness on one native Linux host.

Uses preloaded identical base images, synthetic auth and unique scratch resources.
Docker is only the comparison baseline; it is not a product dependency.
"""
import argparse
import hashlib
import http.client
import json
import os
from pathlib import Path
import platform
import socket
import statistics
import subprocess
import tempfile
import time
import urllib.parse
import uuid


class NodeConnection(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__('localhost', timeout=180)
        self.path = path
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.path)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--cli', type=Path, required=True)
    p.add_argument('--image', default='codex-dock:kata-guard')
    p.add_argument('--proxy-image', default='codex-dock-proxy:policy-verification')
    p.add_argument('--output', type=Path, required=True)
    p.add_argument('--iterations', type=int, default=3)
    p.add_argument('--sudo-docker', action='store_true')
    args = p.parse_args()
    if not 3 <= args.iterations <= 10 or os.getuid() == 0:
        raise SystemExit('run as the normal node operator, with 3..10 iterations')
    args.cli = args.cli.resolve()
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=False)
    docker_env = dict(os.environ)
    for key in ['DOCKER_HOST', 'DOCKER_CONTEXT', 'DOCKER_TLS', 'DOCKER_TLS_VERIFY', 'DOCKER_CERT_PATH']:
        docker_env.pop(key, None)
    docker = (['sudo', '-n'] if args.sudo_docker else []) + ['docker', '--host=unix:///var/run/docker.sock']
    prefix = 'dock-bench-' + uuid.uuid4().hex[:12]
    results = {'schema': 1, 'host_kernel': platform.release(), 'iterations': args.iterations,
               'scope': 'preloaded base, unique COPY+Python RUN; end-to-end CLI readiness with proxy already running, not pure VMM boot latency',
               'builds': {}, 'workers': {}, 'oci': {}}
    with tempfile.TemporaryDirectory(prefix='codex-dock-benchmark-') as tmp:
        home = Path(tmp)
        env = dict(os.environ, HOME=tmp, XDG_CONFIG_HOME=str(home/'config'), XDG_STATE_HOME=str(home/'xdg-state'),
                   CODEX_DOCK_STATE_DIR=str(home/'state'), OPENAI_API_KEY='sk-benchmark-fixture-never-valid', ANTHROPIC_API_KEY='')
        logs = args.output
        sequence = 0
        def run(argv, label, cwd=home, check=True, docker_call=False):
            nonlocal sequence
            sequence += 1
            start = time.monotonic()
            result = subprocess.run([str(x) for x in argv], cwd=cwd, env=docker_env if docker_call else env,
                                    capture_output=True, text=True, timeout=240)
            text = result.stdout + result.stderr
            (logs/f'{sequence:02d}-{label}.log').write_text(text)
            if check and result.returncode:
                raise RuntimeError(f'{label} failed ({result.returncode}); inspect its log')
            return time.monotonic()-start, text, result.returncode
        config = home/'config.toml'
        with socket.socket() as listener:
            listener.bind(('127.0.0.1',0)); port=listener.getsockname()[1]
        auth, http, network = prefix+'-auth',prefix+'-http',prefix+'-egress'
        config.write_text(f'proxy_image = {json.dumps(args.proxy_image)}\nauto_build = false\n[proxy]\nadmin_url = "http://127.0.0.1:{port}"\ncontainer_url = "http://{auth}:18080"\nadmin_secret = "benchmark-fixture"\n')
        config.chmod(0o600)
        cli = [args.cli,'--config',config]
        node_socket = os.environ.get('CODEX_DOCK_KATA_NODE_SOCKET','/run/codex-dock-node.sock')
        active_workers=[]
        docker_tags=[]
        try:
            _,text,_=run(docker+['info','--format','{{.ServerVersion}} {{.Driver}}'], 'docker-info',docker_call=True)
            results['docker']=text.strip()
            _,text,_=run(cli+['runtime-check','--json'],'runtime-check')
            results['readiness']=json.loads(text)
            _,text,_=run(cli+['image','check',args.image,'--agent','codex','--json'],'image-check')
            report=json.loads(text)
            config_digest=next(c['detail'] for c in report['checks'] if c['name']=='identity')
            _,docker_digest,_=run(docker+['image','inspect','--format','{{.Id}}',args.image],'base-image',docker_call=True)
            if docker_digest.strip()!=config_digest:raise RuntimeError('Docker and Kata base images differ')
            results['base_config_digest']=config_digest
            run(cli+['proxy','run','--name',auth,'--http-name',http,'--network',network,'--admin-port',str(port),'--admin-secret','benchmark-fixture'],'proxy')
            context=home/'build';context.mkdir()
            (context/'marker').write_text(prefix)
            (context/'Dockerfile').write_text(f'FROM {args.image}\nCOPY marker /opt/benchmark-marker\nRUN python3 -c "import hashlib; print(hashlib.sha256(open(\'/opt/benchmark-marker\',\'rb\').read()).hexdigest())"\n')
            for kind in ['docker','kata']:
                tag=prefix+'-'+kind+':test'
                if kind=='docker':docker_tags.append(tag)
                samples=[]
                for i in range(args.iterations):
                    argv=docker+['build','--network=none','--progress=plain','-t',tag,'.'] if kind=='docker' else cli+['build','-f','Dockerfile','--tag',tag]
                    elapsed,text,_=run(argv,f'build-{kind}-{i}',context,docker_call=kind=='docker')
                    if i > 0 and 'CACHED' not in text:raise RuntimeError(kind+' rebuild did not report cache reuse')
                    samples.append({'seconds':elapsed,'cached_steps':text.count('CACHED')})
                results['builds'][kind]=samples
            archive=args.output/'cached-export.tar'
            builder=os.environ.get('BUILDKIT_HOST','unix:///run/codex-dock-buildkit/buildkitd.sock')
            elapsed,_,_=run(['buildctl','--addr',builder,'build','--progress=plain','--frontend','dockerfile.v0','--local','context='+str(context),'--local','dockerfile='+str(context),'--opt','image-resolve-mode=local','--output',f'type=oci,name={prefix}-kata:test,dest={archive}'],'cached-oci-export')
            results['oci']['cached_export_seconds']=elapsed
            results['oci']['archive_bytes']=archive.stat().st_size
            connection=NodeConnection(node_socket)
            start=time.monotonic()
            with archive.open('rb') as stream:
                connection.request('POST','/images/load',body=stream,headers={'Content-Length':str(archive.stat().st_size),'Content-Type':'application/x-tar'})
                response=connection.getresponse();response.read()
                if response.status!=200:raise RuntimeError('cached OCI import failed')
            connection.close()
            results['oci']['cached_import_seconds']=time.monotonic()-start
            archive.unlink()
            project=home/'project';project.mkdir()
            host_boot=Path('/proc/sys/kernel/random/boot_id').read_text().strip()
            for kind in ['docker','kata']:
                samples=[]
                for i in range(args.iterations):
                    name=prefix+'-'+kind+'-'+str(i)
                    control=home/name;control.mkdir()
                    probe='import os,time,json;from pathlib import Path;Path("/control/ready").write_text(json.dumps({"uid":os.getuid(),"boot_id":Path("/proc/sys/kernel/random/boot_id").read_text().strip()}));time.sleep(300)'
                    command='exec python3 -c '+__import__('shlex').quote(probe)
                    if kind=='docker':
                        argv=docker+['run','-d','--name',name,'--network=none','--cap-drop=ALL','--security-opt=no-new-privileges','--cpus=1','--memory=2g','--memory-swap=2g','--pids-limit=512','--user',f'{os.getuid()}:{os.getgid()}','--workdir','/workspace','--mount',f'type=bind,src={project},dst=/workspace','--mount',f'type=bind,src={control},dst=/control','-e','DOCK_EXEC_CMD='+command,args.image]
                    else:
                        argv=cli+['run','--runtime','kata','--name',name,'--image',args.image,'--project',project,'--detach','--keep','--no-internet','--http-proxy-url',f'http://{http}:18082','--mount',f'{control}:/control:rw','--exec',command]
                    active_workers.append((kind,name))
                    start=time.monotonic()
                    run(argv,f'start-{kind}-{i}',docker_call=kind=='docker')
                    deadline=time.monotonic()+30
                    while True:
                        try:
                            ready=json.loads((control/'ready').read_text())
                            break
                        except (FileNotFoundError,json.JSONDecodeError):
                            if time.monotonic()>deadline:raise RuntimeError('worker readiness timeout')
                            time.sleep(.01)
                    if ready['uid']!=os.getuid() or (ready['boot_id']==host_boot)!=(kind=='docker'):raise RuntimeError('runtime/UID evidence mismatch')
                    sample={'ready_seconds':time.monotonic()-start}
                    stop=docker+['stop','--time','5',name] if kind=='docker' else cli+['stop',name]
                    remove=docker+['rm',name] if kind=='docker' else cli+['rm',name]
                    sample['stop_seconds']=run(stop,f'stop-{kind}-{i}',docker_call=kind=='docker')[0]
                    sample['remove_seconds']=run(remove,f'remove-{kind}-{i}',docker_call=kind=='docker')[0]
                    active_workers.remove((kind,name));samples.append(sample)
                results['workers'][kind]=samples
            (args.output/'results.json').write_text(json.dumps(results,indent=2)+'\n')
            for kind,samples in results['workers'].items():print(kind,'readiness median',round(statistics.median(s['ready_seconds'] for s in samples),3),'s',flush=True)
        finally:
            for kind,name in reversed(active_workers):
                argv=docker+['rm','-f',name] if kind=='docker' else cli+['rm','--force',name]
                run(argv,'cleanup-worker',check=False,docker_call=kind=='docker')
            for name in [http,auth]:run(cli+['proxy','rm','--name',name,'--force'],'cleanup-proxy',check=False)
            connection=NodeConnection(node_socket)
            try:
                connection.request('DELETE','/networks/'+urllib.parse.quote(network,safe=''))
                response=connection.getresponse();response.read()
                if response.status not in [204,404]:raise RuntimeError('benchmark network cleanup failed')
            finally:connection.close()
            for tag in docker_tags:run(docker+['image','rm',tag],'cleanup-image',check=False,docker_call=True)
    print('Results:',args.output/'results.json',flush=True)

if __name__=='__main__':main()
