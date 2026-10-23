#!/usr/bin/env python3
"""Run the current host build through service.sh with a simulator, never a device.

Usage: python3 installer/test-service-smoke.py /absolute/path/to/host-binary
The release verifier is stubbed; this test exercises process/HTTP integration,
not signatures. wget is replaced with a loopback-only Python HTTP reader.
"""
import hashlib
import http.server
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import threading
import urllib.request

binary = Path(sys.argv[1]).resolve()
service = Path(__file__).resolve().parent / 'bundle/service.sh'
with tempfile.TemporaryDirectory(prefix='ufi-service-smoke-') as td:
    base = Path(td)
    root, bundle, tools = [base / p for p in ('root', 'bundle', 'tools')]
    bundle.mkdir()
    tools.mkdir()
    shutil.copy2(binary, bundle / 'ufi-tools-u60pro')
    shutil.copy2(service, bundle / 'service.sh')
    (bundle / 'manifest.json').write_text('{}')
    (bundle / 'public-key.hex').write_text('fixture')
    (bundle / 'verify-release').write_text('#!/bin/sh\nexit 0\n')
    (bundle / 'verify-release').chmod(0o755)
    def checksum():
        files = sorted(p for p in bundle.iterdir() if p.name != 'SHA256SUMS')
        (bundle/'SHA256SUMS').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in files))
    checksum()
    rc_target = base/'original-rc.local'
    rc_target.write_bytes(b'#!/bin/sh\n# original system startup\nexit 0')
    rc_target.chmod(0o640)
    original_rc = rc_target.read_bytes()
    rc = base/'rc.local'
    rc.symlink_to(rc_target.name)
    (tools / 'wget').write_text(f'#!{sys.executable}\n' + '''import sys, urllib.request, urllib.parse
url = sys.argv[-1]
assert urllib.parse.urlsplit(url).hostname == '127.0.0.1'
try:
    with urllib.request.urlopen(url, timeout=3) as response:
        sys.stdout.buffer.write(response.read())
except Exception:
    sys.exit(1)
''')
    (tools / 'wget').chmod(0o755)
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        port = s.getsockname()[1]
    env = {k:v for k,v in os.environ.items() if not k.startswith('UFI_')}
    env.update(PATH=str(tools)+os.pathsep+env['PATH'], UFI_INSTALL_ROOT=str(root),
               UFI_HTTP_ADDR=f'127.0.0.1:{port}', UFI_AUTOSTART='1', UFI_RC_LOCAL=str(rc), UFI_HOSTS_PATH='',
               UFI_DEVICE_BACKEND='simulator', UFI_DEVICE_BASE_URL='', UFI_WORKERS_ENABLED='false',
               UFI_AT_ENABLED='false', UFI_PRIVILEGED_COMPAT='false',
               UFI_PLUGIN_STORAGE_ENABLED='false', UFI_STAGING_DIR=str(base/'staging'),
               UFI_HEALTH_ATTEMPTS='5', UFI_STOP_ATTEMPTS='3')
    def run(*args):
        subprocess.run(['sh', str(service), *args], env=env, check=True, timeout=45)
    def identity():
        with urllib.request.urlopen(f'http://127.0.0.1:{port}/healthz', timeout=3) as response:
            data = json.load(response)['data']
        assert data['ok'] is True
        assert data['instance'] == (root/'.ufi-health-instance').read_text().strip()
        return data['instance']
    try:
        class ForeignHealth(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b'{"data":{"ok":true,"instance":"foreign"}}')
            def log_message(self, *args):
                pass
        foreign_server = http.server.HTTPServer(('127.0.0.1', 0), ForeignHealth)
        foreign_thread = threading.Thread(target=foreign_server.serve_forever, daemon=True)
        foreign_thread.start()
        try:
            busy_env = dict(env, UFI_HTTP_ADDR=f'127.0.0.1:{foreign_server.server_port}', UFI_HEALTH_ATTEMPTS='1')
            busy = subprocess.run(['sh',str(service),'install',str(bundle)],env=busy_env,timeout=30)
            assert busy.returncode != 0, 'occupied port accepted'
            with urllib.request.urlopen(f'http://127.0.0.1:{foreign_server.server_port}/healthz') as r:
                assert json.load(r)['data']['instance'] == 'foreign'
            assert rc_target.read_bytes() == original_rc
        finally:
            foreign_server.shutdown()
            foreign_server.server_close()
            foreign_thread.join()
        run('install', str(bundle))
        assert root.stat().st_mode & 0o777 == 0o700
        assert (root/'.ufi-system').stat().st_mode & 0o777 == 0o700
        first = identity()
        assert rc.is_symlink() and rc_target.stat().st_mode & 0o777 == 0o640
        run('stop')
        foreign = subprocess.Popen(['sleep', '120'])
        try:
            (root/'ufi-tools.pid').write_text(str(foreign.pid))
            run('stop')
            assert foreign.poll() is None, 'foreign PID was killed'
        finally:
            foreign.terminate()
            foreign.wait()
        cold_env = {k:v for k,v in env.items() if not k.startswith('UFI_')}
        # Boot-style start: no root/port/backend overrides; load the saved data.
        subprocess.run(['sh',str(root/'service.sh'),'start'],env=cold_env,check=True,timeout=30)
        run('health')
        assert identity() != first
        (tools/'df').write_text('#!/bin/sh\nprintf "Filesystem 1024-blocks Used Available Capacity Mounted on\\nfixture 10 9 1 99%% /\\n"\n')
        (tools/'df').chmod(0o755)
        full = subprocess.run(['sh',str(service),'install',str(bundle)],env=env,timeout=30)
        assert full.returncode != 0, 'insufficient disk space accepted'
        (tools/'df').unlink()
        identity()
        shipped_script = (bundle/'service.sh').read_bytes()
        (bundle/'service.sh').write_bytes(shipped_script+b'\n# unexpected change\n')
        bad = subprocess.run(['sh',str(service),'install',str(bundle)],env=env,timeout=30)
        assert bad.returncode != 0, 'tampered service script accepted'
        identity()  # active instance still healthy after verifier rejection
        (bundle/'service.sh').write_bytes(shipped_script)
        (root/'data/preservation-marker').write_text('keep me')
        run('install', str(bundle))
        second = identity()
        assert first != second, 'restart reused process identity'
        assert (root/'data/preservation-marker').read_text() == 'keep me'
        run('uninstall')
        assert not root.exists()
        assert (base/'root.uninstalled/data/preservation-marker').read_text() == 'keep me'
        assert rc.is_symlink() and rc_target.read_bytes() == original_rc, 'rc.local was not restored byte-for-byte'
        assert rc_target.stat().st_mode & 0o777 == 0o640
        run('uninstall','--restore')
        identity()
        rc_target.write_text(rc_target.read_text()+'\n# later user change\n')
        expected = rc_target.read_text().replace(f'sh {root}/service.sh start\n','')
        run('uninstall','--keep-data')
        assert rc_target.read_text() == expected, 'later user edit was lost'
        run('uninstall','--purge')
        assert rc_target.read_text() == expected
        assert not root.exists()
        run('uninstall')
        print('real host install/upgrade/cold-start, foreign PID protection, checksum rejection, rc.local restoration and uninstall passed')
    finally:
        subprocess.run(['sh', str(service), 'stop'], env=env,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=30)
