#!/usr/bin/env bash
set -euo pipefail
example_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
geblang_bin=$(command -v "${GEBLANG_BIN:-geblang}")
exec python3 - "$example_dir" "$geblang_bin" <<'PY'
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

example = Path(sys.argv[1]).resolve()
engine = str(Path(sys.argv[2]).resolve())
framework = example.parent.parent


def run(args, cwd, env=None, expected=None):
    result = subprocess.run(args, cwd=cwd, env=env, text=True, capture_output=True, timeout=180)
    if result.returncode or (expected is not None and result.stdout != expected):
        raise RuntimeError(f"command failed: {args}\n{result.stdout}\n{result.stderr}")
    return result.stdout


with tempfile.TemporaryDirectory(prefix="gebweb-auth-") as scratch:
    root = Path(scratch)
    project = root / "project"
    shutil.copytree(example / "src", project / "src")
    shutil.copytree(example / "tests", project / "tests")
    (project / "geblang.yaml").write_text(
        "name: authentication_verification\nversion: 0.0.0\nsource: src\n"
        f"dependencies:\n  gebweb:\n    path: {framework}\n"
    )
    run([engine, "check", "--strict", "src/"], project)
    for runtime in [[], ["--runtime=evaluator"]]:
        run([engine, "test", *runtime, "tests/"], project)

    help_environment = {key: value for key, value in os.environ.items()
                        if not key.startswith("GEBWEB_") and key not in
                        {"AUTH_USERNAME", "AUTH_PASSWORD_HASH", "JWT_SECRET"}}
    for lane in [[], ["--disable-vm"]]:
        output = run([engine, *lane, "-m", "main", "--help"], project, env=help_environment)
        assert "authentication server options" in output

    fixtures = project / "src" / "fixtures"
    fixtures.mkdir()
    shutil.copy(framework / "tests/fixtures/auth_extension.gb", fixtures)
    shutil.copy(framework / "tests/fixtures/probe.gb", project / "src/probe.gb")
    for args in [["-m", "probe"], ["-m", "probe"], ["--disable-vm", "-m", "probe"]]:
        run([engine, *args], project, expected="extensions-ok\n")
    probe = root / "extension-probe"
    run([engine, "build", "--entry", "probe", "--out", str(probe), "."], project)
    run([str(probe)], root, expected="extensions-ok\n")

    credentials = project / "credentials.gb"
    credentials.write_text('import crypt; import io; io.println(crypt.passwordHash("verification-password"));\n')
    password_hash = run([engine, str(credentials)], project).strip()
    credentials.unlink()
    environment = dict(os.environ, AUTH_USERNAME="ada", AUTH_PASSWORD_HASH=password_hash,
                       JWT_SECRET="verification-only-key-at-least-32-bytes")

    def verify_server(command):
        with socket.socket() as lease:
            lease.bind(("127.0.0.1", 0))
            port = lease.getsockname()[1]
        for name in list(environment):
            if name.startswith("GEBWEB_"):
                del environment[name]
        environment.update(GEBWEB_HOST="127.0.0.1", GEBWEB_PORT=str(port), GEBWEB_NO_TLS="1")
        base = f"http://127.0.0.1:{port}"
        with (root / "server.log").open("w+") as log:
            process = subprocess.Popen(command, cwd=project, env=environment, stdout=log, stderr=log)
            try:
                deadline = time.monotonic() + 30
                while True:
                    if process.poll() is not None:
                        raise RuntimeError("server exited before readiness")
                    try:
                        urllib.request.urlopen(base + "/me", timeout=1).close()
                        break
                    except urllib.error.HTTPError:
                        break
                    except urllib.error.URLError:
                        if time.monotonic() >= deadline:
                            raise RuntimeError("server readiness deadline exceeded")
                        time.sleep(0.05)

                def request(path, body=None, token=None):
                    headers = {"Content-Type": "application/json"}
                    if token is not None:
                        headers["Authorization"] = "Bearer " + token
                    data = None if body is None else json.dumps(body).encode()
                    req = urllib.request.Request(base + path, data=data, headers=headers)
                    try:
                        response = urllib.request.urlopen(req, timeout=5)
                    except urllib.error.HTTPError as error:
                        response = error
                    with response:
                        payload = response.read()
                        return response.status, response.headers, payload

                assert request("/me")[0] == 401
                assert request("/login", {"username": "ada", "password": "wrong"})[0] == 401
                status, headers, body = request("/login", {"username": "ada", "password": "verification-password"})
                assert status == 200 and headers["Cache-Control"] == "no-store"
                token = json.loads(body)["access_token"]
                status, headers, body = request("/me", token=token)
                assert status == 200 and json.loads(body)["name"] == "ada"
                assert headers["X-Content-Type-Options"] == "nosniff"
                assert request("/me", token="broken")[0] == 401
                assert request("/admin", token=token)[0] == 403
            except BaseException:
                log.flush()
                log.seek(0)
                print(log.read(), file=sys.stderr)
                raise
            finally:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)

    for args in [["-m", "main"], ["-m", "main"], ["--disable-vm", "-m", "main"]]:
        verify_server([engine, *args])
    executable = root / "authentication-app"
    run([engine, "build", "--entry", "main", "--out", str(executable), "."], project)
    shutil.rmtree(project / "src")
    shutil.rmtree(project / "tests")
    verify_server([str(executable)])
    print("authentication: VM, cached VM, evaluator, imported extensions and standalone binaries passed")
PY
