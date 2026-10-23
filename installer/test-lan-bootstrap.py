#!/usr/bin/env python3
"""Exercise LAN boot operations with a host simulator and temporary files only.
The vendor restore step is simulated by setting the prepared boot-file content.
No reference binary or real device is executed.
"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import urllib.request

service = Path(__file__).resolve().parent/'bundle/service.sh'
binary = Path(sys.argv[1]).resolve()
with tempfile.TemporaryDirectory(prefix='ufi-lan-bootstrap-') as td:
    base = Path(td); root = base/'root'; tools=base/'tools'; tools.mkdir()
    rc = base/'rc.local'; original=b'#!/bin/sh\n# original system\nexit 0\n'; rc.write_bytes(original); rc.chmod(0o640)
    (tools/'wget').write_text(f'#!{sys.executable}\n'+'''import sys,urllib.request,urllib.parse
url=sys.argv[-1]
assert urllib.parse.urlsplit(url).hostname=='127.0.0.1'
try:
    with urllib.request.urlopen(url,timeout=2) as r: sys.stdout.buffer.write(r.read())
except Exception: sys.exit(1)
'''); (tools/'wget').chmod(0o755)
    with socket.socket() as sock:
        sock.bind(('127.0.0.1',0)); port=sock.getsockname()[1]
    env={k:v for k,v in os.environ.items() if not k.startswith('UFI_')}
    env.update(PATH=str(tools)+os.pathsep+env['PATH'],UFI_INSTALL_ROOT=str(root),UFI_RC_LOCAL=str(rc),
               UFI_HTTP_ADDR=f'127.0.0.1:{port}',UFI_DEVICE_BACKEND='simulator',UFI_DEVICE_BASE_URL='',
               UFI_HEALTH_ATTEMPTS='3',UFI_STOP_ATTEMPTS='3')
    deployment='a'*32
    def stage(action,identity,tag,before):
        folder=Path(str(root)+'.lan-stage-'+tag);folder.mkdir()
        shutil.copy2(service,folder/'service.sh');shutil.copy2(binary,folder/'ufi-tools-u60pro')
        hook=f'sh {folder}/service.sh lan-bootstrap {folder}\n'
        text=before.decode()
        owned=f'sh {root}/service.sh start\n'
        after=text.replace(owned,hook) if owned in text else text.replace('exit 0',hook+'exit 0')
        for name,data in {'lan-plan-id':(identity+'\n').encode(),'lan-action':(action+'\n').encode(),
                          'lan-rc-before':before,'lan-rc-after':after.encode()}.items():
            (folder/name).write_bytes(data)
        if action=='install': (folder/'lan-login-token').write_text('f'*64+'\n')
        (folder/'SHA256SUMS').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in sorted(folder.iterdir())))
        rc.write_text(after) # simulate vendor applying the prepared boot file
        return folder
    def boot(folder,success=True):
        result=subprocess.run(['sh',str(folder/'service.sh'),'lan-bootstrap',str(folder)],env=env,timeout=45)
        assert (result.returncode==0)==success
    def health():
        with urllib.request.urlopen(f'http://127.0.0.1:{port}/healthz',timeout=3) as response: return json.load(response)['data']
    try:
        first=stage('install',deployment,'a'*32,original);boot(first)
        assert not first.exists()
        assert health()['deployment']==deployment
        assert (root/'.ufi-system/before').read_bytes()==original
        assert (root/'lan-plan-id').read_text().strip()==deployment
        request=urllib.request.Request(f'http://127.0.0.1:{port}/api/device/status',headers={'Authorization':'Bearer '+'f'*64})
        with urllib.request.urlopen(request,timeout=3) as response: assert response.status==200
        assert not (root/'data/initial-device-token').exists()
        installed_rc=rc.read_bytes()
        occupied=stage('install','e'*32,'e'*32,installed_rc);boot(occupied,False)
        assert health()['deployment']==deployment and rc.read_bytes()==installed_rc
        wrong=stage('remove','b'*32,'b'*32,installed_rc);boot(wrong,False)
        assert health()['deployment']==deployment
        assert rc.read_bytes()==installed_rc
        remove=stage('remove',deployment,'c'*32,installed_rc);boot(remove)
        assert not root.exists() and Path(str(root)+'.uninstalled').exists()
        assert rc.read_bytes()==original
        restore=stage('restore',deployment,'d'*32,installed_rc);boot(restore)
        assert health()['deployment']==deployment
        assert rc.read_bytes()==installed_rc
        manual_rc=original+b'# user disabled candidate autostart\n'
        rc.write_bytes(manual_rc)
        remove_again=stage('remove',deployment,'f'*32,manual_rc);boot(remove_again)
        restore_again=stage('restore',deployment,'1'*32,manual_rc);boot(restore_again)
        assert rc.read_bytes()==manual_rc, 'inverse restore re-enabled user-disabled autostart'
        subprocess.run(['sh',str(service),'uninstall','--purge'],env=env,check=True,timeout=30)
        assert rc.read_bytes()==manual_rc and rc.stat().st_mode & 0o777 == 0o640
        print('LAN boot install, receipt mismatch refusal, remove, inverse restore and original boot restoration passed')
    finally:
        subprocess.run(['sh',str(service),'stop'],env=env,timeout=30,check=False)
