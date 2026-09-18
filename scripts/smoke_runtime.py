#!/usr/bin/env python3
"""Exercise the built CLI and optionally Docker with synthetic trace requests.

Run make build and make demo first. Requires only Python's standard library.
Every process/container created here is stopped in finally blocks.
"""

import argparse
import copy
import http.client
import json
from pathlib import Path
import re
import socket
import subprocess
import tempfile
import time
import uuid

ROOT = Path(__file__).resolve().parents[1]


def run(*args):
    return subprocess.run(args, check=True, capture_output=True, text=True, timeout=30).stdout.strip()


def free_ports(number):
    sockets = []
    try:
        for _ in range(number):
            sock = socket.socket()
            sock.bind(("127.0.0.1", 0))
            sockets.append(sock)
        return [sock.getsockname()[1] for sock in sockets]
    finally:
        for sock in sockets:
            sock.close()


def request(port, body=None, content_type="application/json", path="/v1/traces"):
    conn = http.client.HTTPConnection("127.0.0.1", port, timeout=4)
    try:
        conn.request("POST" if body is not None else "GET", path, body,
                     {"Content-Type": content_type} if body is not None else {})
        response = conn.getresponse()
        return response.status, response.read()
    finally:
        conn.close()


def ready(port, alive):
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        if not alive():
            raise RuntimeError("Collector exited before readiness")
        try:
            if request(port, b"{}")[0] == 200:
                return
        except (OSError, http.client.HTTPException):
            pass
        time.sleep(0.05)
    raise TimeoutError("Collector readiness timeout")


def exercise(port, metrics_port, fixture, read_logs):
    checks = []

    def expect(label, body, expected, content_type="application/json", path="/v1/traces"):
        code, _ = request(port, body, content_type, path)
        if code != expected:
            raise AssertionError(f"{label}: HTTP {code}, expected {expected}")
        checks.append(f"{label}: HTTP {code}")

    expect("valid synthetic trace", json.dumps(fixture).encode(), 200)
    bad = copy.deepcopy(fixture)
    spans = bad["resourceSpans"][0]["scopeSpans"][0]["spans"]
    spans[-1]["attributes"].append({"key": "oversize", "value": {"stringValue": "x" * 65}})
    expect("whole-batch policy rejection", json.dumps(bad).encode(), 400)
    expect("malformed JSON", b'{"resourceSpans":', 400)
    expect("unsupported content type", b"{}", 415, "text/plain")
    expect("unsupported logs signal", b"{}", 404, path="/v1/logs")
    expect("HTTP body-size limit", b" " * (4194304 + 1), 400)

    deadline = time.monotonic() + 6
    while time.monotonic() < deadline:
        logs = read_logs()
        if "[REDACTED]" in logs and "inventory.reserve" in logs:
            break
        time.sleep(0.05)
    else:
        raise AssertionError("No verified export observed in debug output")
    if "alice@example.test" in logs or "synthetic-secret-do-not-use" in logs:
        raise AssertionError("Raw synthetic secret reached runtime logs")
    if logs.count("Name           : POST /checkout") != 1:
        raise AssertionError("Expected exactly one exported checkout span")
    checks.append("debug export redacted; original secrets absent; rejected batch not exported")

    if metrics_port is not None:
        code, body = request(metrics_port, path="/metrics")
        if code != 200:
            raise AssertionError("Metrics endpoint unavailable")
        metrics = body.decode()
        for name, value in [("attributes_removed",6), ("values_redacted",8), ("batches_rejected",1)]:
            if not re.search(rf"^otelredactor_{name}(?:_[a-z]+)*(?:\{{[^\n]*\}})? {value}(?:\.0)?$", metrics, re.M):
                raise AssertionError(f"Incorrect runtime counter {name}")
        checks.append("Prometheus counters verified: removed=6, redacted=8, rejected=1")
    return checks


def native(fixture):
    http_port, grpc_port, metrics_port = free_ports(3)
    config = (ROOT / "examples/collector.yaml").read_text()
    for old, new in [(4318,http_port), (4317,grpc_port), (8888,metrics_port)]:
        config = config.replace(str(old),str(new))
    config = config.replace("verbosity: basic", "verbosity: detailed")
    config = config.replace("max_string_bytes: 1048576", "max_string_bytes: 64")
    with tempfile.TemporaryDirectory(prefix="otel-redactor-native-") as directory:
        path = Path(directory) / "config.yaml"
        logs = Path(directory) / "collector.log"
        path.write_text(config)
        with logs.open("w") as out:
            process = subprocess.Popen([str(ROOT/"bin/otel-redactor"), "--config",str(path)], stdout=out, stderr=subprocess.STDOUT)
            try:
                ready(http_port, lambda: process.poll() is None)
                checks = exercise(http_port,metrics_port,fixture,logs.read_text)
            finally:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)
                    raise
            if process.returncode != 0 or "Shutdown complete" not in logs.read_text():
                raise AssertionError("Native SIGTERM shutdown failed")
    return checks + ["native SIGTERM shutdown: exit 0"]


def container(fixture):
    config = (ROOT/"examples/container.yaml").read_text().replace("verbosity: basic", "verbosity: detailed")
    config = config.replace("  otelredactor:\n", "  otelredactor:\n    max_string_bytes: 64\n")
    name = "otel-redactor-smoke-" + uuid.uuid4().hex[:10]
    with tempfile.TemporaryDirectory(prefix="otel-redactor-docker-") as directory:
        path = Path(directory).resolve() / "config.yaml"
        path.write_text(config)
        run("docker","run","--detach","--name",name,"--read-only","--cap-drop","ALL",
            "--security-opt","no-new-privileges","--memory","384m","--cpus","1","--pids-limit","64",
            "-p","127.0.0.1::4318","--mount",f"type=bind,source={path},target=/etc/otel-redactor/config.yaml,readonly",
            "otel-redactor:local")
        try:
            port = int(run("docker","port",name,"4318/tcp").rsplit(":",1)[1])
            ready(port, lambda: run("docker","inspect","--format","{{.State.Running}}",name) == "true")

            def logs():
                result = subprocess.run(["docker","logs",name],check=True,capture_output=True,text=True,timeout=10)
                return result.stdout+result.stderr

            checks = exercise(port,None,fixture,logs)
            state = json.loads(run("docker","inspect",name))[0]
            assert state["Config"]["User"] == "65532:65532"
            assert state["HostConfig"]["ReadonlyRootfs"]
            checks.append("container runs as UID/GID 65532 with read-only filesystem")
            run("docker","stop","--time","10",name)
            assert run("docker","inspect","--format","{{.State.ExitCode}}",name) == "0"
            checks.append("container SIGTERM shutdown: exit 0")
        finally:
            # Only remove the unique container created by this invocation.
            run("docker","rm","--force",name)
    return checks


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--docker",action="store_true",help="also test the locally built Docker image")
    args = parser.parse_args()
    fixture = json.loads((ROOT/"demo-output/before.json").read_text())
    result = {"native":native(fixture)}
    if args.docker:
        result["container"] = container(fixture)
    for runtime, checks in result.items():
        for check in checks:
            print(f"PASS [{runtime}] {check}")
    (ROOT/"demo-output/runtime-checks.json").write_text(json.dumps(result,indent=2)+"\n")


if __name__ == "__main__":
    main()
