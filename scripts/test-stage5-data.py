#!/usr/bin/env python3
"""Run real datad HTTP/SSE -> real DevUI data consumer against fake native data."""
import json
import os
from pathlib import Path
import shlex
import socket
import subprocess
import tempfile
import time
from urllib.request import urlopen

ROOT = Path(__file__).resolve().parents[1]
WORKSPACE = Path(os.environ.get('UFI_PLUGIN_WORKSPACE', ROOT.parent.parent / 'ufitools_plugins'))
DATAD = WORKSPACE / 'core/datad-src'
DEVUI = WORKSPACE / 'core/devui-src'
CC = shlex.split(os.environ.get('CC', 'cc'))

with tempfile.TemporaryDirectory(prefix='ufi-stage5-data-') as raw:
    work = Path(raw)
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    backend = work / 'datad'
    probe = work / 'devui-probe'
    flags = ['-std=c11', '-D_GNU_SOURCE', '-O0', '-Wall', '-Wextra', '-Werror']
    subprocess.run(CC + flags + ['-Wno-unused-parameter', '-Wno-format-truncation', '-I'+str(DATAD/'include')] +
                   list(map(str, sorted((DATAD/'src').glob('*.c')))) + ['-o', str(backend)], check=True)
    subprocess.run(CC + flags + ['-DDEVUI_BACKEND_PORT='+str(port), '-I'+str(DEVUI/'include'),
                   str(ROOT/'scripts/stage5-devui-probe.c'), str(DEVUI/'src/data.c'),
                   str(DEVUI/'src/json.c'), '-o', str(probe)], check=True)
    sim = work/'sim.json'
    traffic = work/'traffic.json'
    sim.write_text(json.dumps({'sim_iccid':'8986000000000012345','current_sim_slot':'1',
                              'sim_imsi':'460001234567890','sim_states':'ready','Operator':'CUCC'}))
    def write_traffic(rx):
        replacement = work/'traffic.new'
        replacement.write_text(json.dumps({'real_rx_bytes':rx,'real_tx_bytes':2000,
                                          'real_rx_speed':100,'real_tx_speed':50,'real_time':10}))
        replacement.replace(traffic)
    write_traffic(1000)
    data = work/'data'
    data.mkdir()
    env = dict(os.environ, PATH=str(DATAD/'tests/fake-bin')+os.pathsep+os.environ['PATH'],
               UBUS_LOG=str(work/'ubus.log'), UBUS_SIM_FILE=str(sim), UBUS_TRAFFIC_FILE=str(traffic),
               ZWRT_DATAD_DATA_DIR=str(data), ZWRT_DATAD_SYSTEM_TIMEZONE_FILE=str(work/'absent-system-timezone'))
    with (work/'datad.log').open('w') as log:
        server = subprocess.Popen([str(backend), '-b', '127.0.0.1', '-p', str(port), '-i', '100'],
                                  env=env, stdout=log, stderr=log)
        consumer = None
        try:
            for _ in range(50):
                try:
                    with urlopen(f'http://127.0.0.1:{port}/healthz', timeout=.5) as response:
                        assert response.read() == b'ok\n'
                    break
                except OSError:
                    if server.poll() is not None:
                        raise RuntimeError((work/'datad.log').read_text())
                    time.sleep(.1)
            else:
                raise RuntimeError('datad readiness timed out')
            consumer = subprocess.Popen([str(probe)], stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                        text=True, env=env)
            first = consumer.stdout.readline().strip()
            if not first.startswith('HTTP_STATE_OK'):
                _, error = consumer.communicate(timeout=7)
                raise RuntimeError(first+' '+error)
            write_traffic(5000)
            rest, error = consumer.communicate(timeout=7)
            assert consumer.returncode == 0, error
            assert 'SSE_STATE_OK' in rest, rest
            print(first)
            print(rest.strip())
            print('DEVUI_DATAD_INTEGRATION_OK (native fixtures; no hardware/display access)')
        finally:
            if consumer is not None and consumer.poll() is None:
                consumer.kill()
                consumer.communicate()
            server.terminate()
            try:
                server.wait(timeout=3)
            except subprocess.TimeoutExpired:
                server.kill()
                server.wait()
