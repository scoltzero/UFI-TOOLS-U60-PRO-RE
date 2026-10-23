#!/usr/bin/env python3
"""Execute the ADB-generated shell locally in a disposable tree, with fake ADB.
No real ADB, mount, device or network operation is performed.
"""
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

project = Path(__file__).resolve().parent.parent
with tempfile.TemporaryDirectory(prefix='ufi-adb-transport-') as td:
    tmp = Path(td)
    bundle, root, tools = [tmp/n for n in ('bundle','root','tools')]
    bundle.mkdir()
    tools.mkdir()
    shutil.copy2(project/'installer/bundle/service.sh', bundle/'service.sh')
    for name in ('install.sh', 'install.command'):
        shutil.copy2(project/name, bundle/name)
    shutil.copy2(project/'installer/install-adb.sh', bundle/'install-adb.sh')
    (bundle/'ufi-tools-u60pro').write_text('#!/bin/sh\ntrap "exit 0" TERM\nwhile :; do sleep 1; done\n')
    (bundle/'ufi-tools-u60pro').chmod(0o755)
    (bundle/'SHA256SUMS').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in sorted(bundle.iterdir())))
    (tools/'adb').write_text(f'#!{sys.executable}\n'+'''
import os, pathlib, shutil, subprocess, sys, shlex, re
with open(os.environ['FAKE_ADB_LOG'],'a') as log: log.write('called\\n')
args=sys.argv[1:]
if args[0]=='-s': args=args[2:]
if args[0]=='get-state': print('device')
elif args[0]=='shell':
    cmd=' '.join(args[1:])
    checked=cmd.startswith("sh -c ")
    payload=shlex.split(cmd)[2] if checked else cmd
    if payload.startswith('id'):
        print('0')
        status=0
    elif payload.startswith('uname'):
        print('aarch64')
        status=0
    else:
        status=subprocess.run(['sh','-c',payload]).returncode
    if checked:
        marker=re.search(r"__UFI_ADB_EXIT_[0-9]+",cmd).group()
        print(chr(10)+marker+'='+str(status),flush=True)
    sys.exit(0) # Model the legacy transport observed on the target.

elif args[0]=='push':
    src,dst=args[1:3]
    if pathlib.Path(src).is_dir(): shutil.copytree(src,dst,dirs_exist_ok=True)
    else: shutil.copy2(src,dst)
else: sys.exit(2)
''')
    (tools/'wget').write_text('''#!/bin/sh
printf '{"data":{"ok":true,"instance":"%s"}}' "$(cat "$UFI_INSTALL_ROOT/.ufi-health-instance")"
''')
    for name in ('adb','wget'):
        (tools/name).chmod(0o755)
    env = {k:v for k,v in os.environ.items() if not k.startswith('UFI_')}
    env.update(ADB=str(tools/'adb'), PATH=str(tools)+os.pathsep+env['PATH'], UFI_HEALTH_ATTEMPTS='1', UFI_STOP_ATTEMPTS='3', FAKE_ADB_LOG=str(tmp/'adb-calls'))
    command = [str(project/'installer/install-adb.sh'),'--execute','--root',str(root),'--no-autostart']
    foreign = tmp/'user-files'
    foreign.mkdir()
    (foreign/'keep').write_text('original contents')
    try:
        entry = ['sh',str(bundle/'install.sh')]
        target = ['--root',str(root),'--no-autostart']
        subprocess.run(entry+['preview']+target,env=env,check=True,timeout=15)
        rejected = subprocess.run(entry+['preview','--execute']+target,env=env,timeout=15)
        assert rejected.returncode != 0
        assert not (tmp/'adb-calls').exists(), 'preview contacted ADB'
        assert not root.exists()
        # A prebuilt package must never invoke a compiler.
        (tools/'go').write_text('#!/bin/sh\nexit 91\n')
        (tools/'go').chmod(0o755)
        subprocess.run(entry+target,env=env,check=True,timeout=45)
        assert (root/'.ufi-runtime').is_file()
        assert not Path(str(root)+'.staging').exists()
        subprocess.run(command+['--bundle',str(bundle)],env=env,check=True,timeout=45)
        collision = subprocess.run(command+['--bundle',str(bundle),'--staging',str(foreign)],env=env,timeout=30)
        assert collision.returncode != 0
        assert (foreign/'keep').read_text() == 'original contents'
        # An owner symlink must be rejected before prepare_staging writes to it.
        linked_stage = tmp/'linked-stage'
        protected = tmp/'protected-owner-target'
        protected.write_text('user-owned data')
        Path(str(linked_stage)+'.owner').symlink_to(protected)
        unsafe = subprocess.run(command+['--bundle',str(bundle),'--staging',str(linked_stage)],env=env,timeout=30)
        assert unsafe.returncode != 0
        assert protected.read_text() == 'user-owned data'
        assert not linked_stage.exists()
        subprocess.run(entry+['uninstall']+target,env=env,check=True,timeout=45)
        assert not root.exists()
        assert Path(str(root)+'.uninstalled/.ufi-tools-manifest').is_file()
        assert (foreign/'keep').read_text() == 'original contents'
        assert not Path(str(root)+'.staging.controller').exists()
        subprocess.run(command+['--uninstall'],env=env,check=True,timeout=45)
        subprocess.run(command+['--uninstall','--purge'],env=env,check=True,timeout=45)
        assert not Path(str(root)+'.previous').exists()
        assert not Path(str(root)+'.uninstalled').exists()
        print('ADB shell transport install/uninstall and staging ownership passed')
    finally:
        subprocess.run(['sh',str(project/'installer/bundle/service.sh'),'stop'],
                       env=dict(env,UFI_INSTALL_ROOT=str(root)),timeout=30,check=False)
