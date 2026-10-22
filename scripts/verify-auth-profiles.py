#!/usr/bin/env python3
"""Check the real profile CLI and proxy listener using synthetic credentials only."""
import argparse
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
from urllib.error import HTTPError, URLError
from urllib.request import Request, build_opener, ProxyHandler


def port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    args = parser.parse_args()
    binary = str(Path(args.binary).resolve())
    key = "fixture-product-key-stays-on-host"
    host_key = "fixture-host-key-must-not-be-selected"
    secret = "fixture-admin-" * 4
    # Explicitly local test I/O; no external API, login or inference requests.
    opener = build_opener(ProxyHandler({}))
    with tempfile.TemporaryDirectory(prefix="cxd-profile-acceptance-") as temporary:
        root = Path(temporary)
        admin_file = root / "admin-secret"
        with os.fdopen(os.open(admin_file, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "w") as file:
            file.write(secret + "\n")
        home, profiles = root / "home", root / "profiles"
        (home / ".codex").mkdir(parents=True, mode=0o700)
        (home / ".codex/auth.json").write_text(json.dumps({
            "auth_mode": "chatgpt", "tokens": {"access_token": host_key, "refresh_token": "fixture-host-refresh"}}))
        env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "HOME": str(home),
               "OPENAI_API_KEY": host_key, "ANTHROPIC_API_KEY": host_key,
               "CLAUDE_CODE_OAUTH_TOKEN": host_key}
        base = [binary, "--auth-profile-dir", str(profiles)]

        def run(arguments, input=None):
            result = subprocess.run(base + arguments, input=input, text=True, stdout=subprocess.PIPE,
                                    stderr=subprocess.STDOUT, env=env, timeout=10, check=False)
            assert key not in result.stdout and host_key not in result.stdout, "credential leaked to CLI output"
            assert result.returncode == 0, "CLI operation failed"
            return result.stdout

        run(["auth", "profile", "set", "smoke", "--key-stdin"], key + "\n")
        output = run(["--auth-profile", "smoke", "auth", "show"])
        assert "Profile: smoke" in output and "api-key" in output
        profile = profiles / "smoke/smoke.json"
        assert profile.stat().st_mode & 0o777 == 0o600
        assert profile.parent.stat().st_mode & 0o777 == 0o700
        record = json.loads(profile.read_text())
        assert record["credentials"]["openai_api_key"] == key
        data_port, admin_port = port(), port()
        while admin_port == data_port:
            admin_port = port()
        command = base + ["--auth-profile", "smoke", "proxy", "serve", "--listen", f"127.0.0.1:{data_port}",
                          "--admin-listen", f"127.0.0.1:{admin_port}", "--admin-secret-file", str(admin_file)]
        process = subprocess.Popen(command, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        try:
            admin = f"http://127.0.0.1:{admin_port}"
            headers = {"X-Proxy-Admin-Secret": secret}
            deadline = time.monotonic() + 10
            while True:
                assert process.poll() is None, "profile proxy exited before readiness"
                try:
                    with opener.open(Request(admin + "/admin/mode", headers=headers), timeout=1) as response:
                        mode = json.load(response)
                        assert response.headers["X-Cxd-Auth-Profile"] == "smoke"
                        assert response.headers["X-Cxd-Connection-Version"] == record["credentials"]["connection_version"]
                    break
                except URLError:
                    assert time.monotonic() < deadline, "profile proxy did not become ready"
                    time.sleep(0.05)
            assert mode["independent_profile"] and not mode["oauth_mode"] and not mode["anthropic_available"], "host auth was selected"
            request = Request(admin + "/admin/issue", method="POST", headers={**headers, "Content-Type": "application/json"},
                              data=json.dumps({"container": "smoke-worker", "ttl": 60}).encode())
            with opener.open(request, timeout=3) as response:
                token = json.load(response)["token"]
            request = Request(f"http://127.0.0.1:{data_port}/token", headers={"X-Codex-Token": token})
            with opener.open(request, timeout=3) as response:
                body = response.read().decode()
                assert json.loads(body)["api_key"] == token
                assert key not in body and host_key not in body, "credential leaked to worker response"
            with opener.open(Request(admin + "/admin/revoke?container=smoke-worker", method="POST", headers=headers), timeout=3):
                pass
            try:
                opener.open(request, timeout=3)
                raise AssertionError("revoked worker token accepted")
            except HTTPError as error:
                assert error.code == 401
            missing = subprocess.run(base + ["--auth-profile", "missing", "proxy", "serve", "--listen", f"127.0.0.1:{port()}",
                                             "--admin-listen", f"127.0.0.1:{port()}", "--admin-secret-file", str(admin_file)],
                                     env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=3, check=False)
            assert missing.returncode != 0 and "credential connection not found" in missing.stdout, "missing profile did not fail closed"
            assert key not in missing.stdout and host_key not in missing.stdout
            run(["auth", "profile", "list"])
        finally:
            process.terminate()
            try:
                output, _ = process.communicate(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                output, _ = process.communicate()
            assert key not in output and host_key not in output, "credential leaked to proxy logs"
    print("PASS: profile CLI, 0700/0600 storage, isolated proxy startup, placeholders, revocation and no host fallback")


if __name__ == "__main__":
    main()
