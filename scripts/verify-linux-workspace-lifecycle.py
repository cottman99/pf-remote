"""Cold-start and reconnect replicas; never attach to a real desktop/audio core.

Uses the installed VNC/RDP/WM/audio implementations, temporary local listeners,
private audio runtimes, WirePlumber's policy-only profile and generated audio.
"""
import argparse
import configparser
import ctypes as C
import hashlib
import importlib.util
import json
import math
import os
from pathlib import Path
import signal
import shutil
import socket
import struct
import subprocess
import sys
import tempfile
import time


def load(name, file):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(file))
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


input_test = load('input_test', 'verify-linux-workspace-input.py')
router = load('audio_router', 'linux-workspace-audio-router.py')


def run(args, **kwargs):
    return subprocess.run(args, check=True, capture_output=True, timeout=20, **kwargs).stdout


def stop(process):
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    if process.poll() is None:
        try:
            process.wait(timeout=4)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=4)


def free_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def wait_socket(path, process):
    deadline = time.monotonic() + 10
    while not path.is_socket():
        if process.poll() is not None or time.monotonic() >= deadline:
            raise RuntimeError('isolated audio core did not open its socket')
        time.sleep(.05)


def verify_mapping(workspace, vnc_binary):
    """Read actual mapping; lifecycle actions still use disposable listeners."""
    config = configparser.ConfigParser(interpolation=None, strict=False)
    config.read('/etc/xrdp/xrdp.ini')
    section = config['pfremote-shared-' + str(workspace)]
    if (section.get('ip') != '127.0.0.1' or section.getint('port') != 5900 + workspace or
            section.get('chansrvport') != f'DISPLAY({workspace},{os.getuid()})'):
        raise ValueError('existing workspace mapping differs from the tested model')
    pid = run(['systemctl', '--user', 'show', f'vnc-display@{workspace}.service',
               '-p', 'MainPID', '--value']).decode().strip()
    if Path('/proc', pid, 'exe').resolve() != Path(shutil.which(vnc_binary) or vnc_binary).resolve():
        raise ValueError('workspace uses a different VNC implementation')
    command = run(['systemctl', '--user', 'show', f'pfremote-shared-desktop-{workspace}-audio.service',
                   '-p', 'ExecStart', '-p', 'Environment']).decode()
    if not all(value in command for value in [f'node.name=pfremote-mic-{workspace}',
            f'node.name=pfremote-speaker-{workspace}', 'priority.session=100',
            'node.always-process=true', f'xrdp_chansrv_audio_in_socket_{workspace}']):
        raise ValueError('workspace audio configuration differs from the tested model')
    return True


def tone():
    return b''.join(struct.pack('<hh', *([int(5000 * math.sin(2 * math.pi * 997 * i / 44100))] * 2))
                    for i in range(44100 * 6))


def measure(data):
    values = struct.unpack('<' + 'h' * (len(data) // 2), data[:len(data) // 2 * 2])[::2]
    signal_indices = [i for i, value in enumerate(values) if abs(value) > 50]
    if not signal_indices:
        return {'passed': False, 'tone_seconds': 0}
    values = values[signal_indices[0]:signal_indices[-1] + 1]
    edges = [i for i in range(len(values) - 1) if values[i] <= 0 < values[i + 1]]
    periods = [b - a for a, b in zip(edges, edges[1:]) if 30 <= b - a <= 60]
    pitch = 44100 * len(periods) / sum(periods) if periods else 0
    longest = current = steady = startup = 0
    for index, value in enumerate(values):
        current = current + 1 if abs(value) < 50 else 0
        longest = max(longest, current)
        if index < 44100:
            startup = max(startup, current)
        else:
            steady = max(steady, current)
    duration = len(values) / 44100
    # A freshly clocked virtual capture path can negotiate one 21-ms buffer
    # near onset. Report it separately; reject >50-ms startup gaps and >10-ms
    # gaps after the first second. Do not hide missing or discontinuous audio.
    return {'passed': duration > 5.5 and abs(pitch - 997) < 5 and startup < 2205 and steady < 441,
            'tone_seconds': round(duration, 2), 'tone_hz': round(pitch, 1),
            'longest_silence_ms': round(longest / 44.1, 2),
            'startup_silence_ms': round(startup / 44.1, 2),
            'steady_silence_ms': round(steady / 44.1, 2)}


def native_client_window(x):
    x.x.XQueryTree.argtypes = [C.c_void_p, C.c_ulong, C.POINTER(C.c_ulong),
        C.POINTER(C.c_ulong), C.POINTER(C.POINTER(C.c_ulong)), C.POINTER(C.c_uint)]
    root, parent, children, count = C.c_ulong(), C.c_ulong(), C.POINTER(C.c_ulong)(), C.c_uint()
    x.x.XQueryTree(x.d, x.root, C.byref(root), C.byref(parent), C.byref(children), C.byref(count))
    try:
        candidates = []
        for window in list(children[:count.value]):
            attrs = x.attributes(window)
            if attrs and attrs.map_state == 2 and attrs.width >= 500 and attrs.height >= 400:
                candidates.append(window)
        if len(candidates) != 1:
            raise ValueError('isolated client window is not unique')
        return candidates[0]
    finally:
        if children:
            x.x.XFree(children)


def replica(workspace, generation, vnc_binary, base):
    report = {'workspace': workspace, 'cold_start': generation, 'connections': []}
    report['live_mapping_matches_test_model'] = verify_mapping(workspace, vnc_binary)
    children, window, client_x = [], None, None
    # Keep failure diagnostics private; successful generated artifacts are removed.
    # Unix-domain socket names, including workspace links, must fit 108 bytes.
    short_base = Path.home() / '.cache/pfr-verify'
    short_base.mkdir(mode=0o700, parents=True, exist_ok=True)
    if not router.protected_directory(short_base, os.getuid()):
        raise ValueError('private verification directory is not protected')
    directory = tempfile.mkdtemp(prefix='lc-', dir=short_base)
    root = Path(directory)
    runtime = root / 'runtime'
    runtime.mkdir(mode=0o700)
    displays = [d for d in range(80, 100) if not Path(f'/tmp/.X11-unix/X{d}').exists()
                and not Path(f'/tmp/.X{d}-lock').exists()]
    if len(displays) < 2:
        raise ValueError('no two disposable displays available')
    display, client_display = displays[:2]
    (runtime / f'vnc-{display}').mkdir(mode=0o700)
    auth = root / 'Xauthority'
    auth.touch(mode=0o600)
    for d in (display, client_display):
        run(['xauth', '-f', str(auth), 'add', ':' + str(d), 'MIT-MAGIC-COOKIE-1', os.urandom(16).hex()])
    env = dict(os.environ, DISPLAY=':' + str(display), XAUTHORITY=str(auth),
        XDG_RUNTIME_DIR=str(runtime), PIPEWIRE_RUNTIME_DIR=str(runtime), PIPEWIRE_REMOTE='pipewire-0',
        XDG_CONFIG_HOME=str(root / 'config'), XDG_CACHE_HOME=str(root / 'cache'),
        XDG_STATE_HOME=str(root / 'state'), PULSE_SERVER='unix:' + str(runtime / 'pulse/native'),
        GIO_USE_VFS='local', NO_AT_BRIDGE='1',
        XRDP_SOCKET_PATH='/run/xrdp/' + str(os.getuid()),
        XRDP_PULSE_SINK_SOCKET=f'xrdp_chansrv_audio_out_socket_{display}',
        XRDP_PULSE_SOURCE_SOCKET=f'xrdp_chansrv_audio_in_socket_{display}')
    env.pop('DBUS_SESSION_BUS_ADDRESS', None)
    env.pop('SESSION_MANAGER', None)
    client_env = dict(env, DISPLAY=':' + str(client_display))
    def start(args, name, environment=env, **kwargs):
        with (root / (name + '.log')).open('ab') as log:
            process = subprocess.Popen(args, env=environment, stdout=log,
                stderr=subprocess.STDOUT, start_new_session=True, **kwargs)
        children.append(process)
        return process
    previous_auth = os.environ.get('XAUTHORITY')
    try:
        secret = os.urandom(4).hex().encode()
        encrypted = run(['openssl', 'enc', '-des-ecb', '-K', 'e84ad660c4721ae0', '-nopad', '-provider', 'legacy'], input=secret)
        credential = root / 'vnc.pass'
        credential.write_bytes(encrypted)
        credential.chmod(0o600)
        run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-keyout', str(root / 'key.pem'),
             '-out', str(root / 'cert.pem'), '-days', '1', '-subj', '/CN=PF Remote isolated lifecycle'])
        (root / 'key.pem').chmod(0o600)
        fingerprint = hashlib.sha256(run(['openssl', 'x509', '-in', str(root / 'cert.pem'), '-outform', 'DER'])).hexdigest()
        fingerprint = ':'.join(fingerprint[i:i + 2] for i in range(0, len(fingerprint), 2))
        vnc_port, rdp_port = free_port(), free_port()
        config = f'''[Globals]
ini_version=1
fork=true
port=tcp://127.0.0.1:{rdp_port}
security_layer=tls
crypt_level=high
certificate={root}/cert.pem
key_file={root}/key.pem
autorun=replica
allow_channels=true
max_bpp=32
use_fastpath=both
[Logging]
LogFile={root}/xrdp.log
LogLevel=INFO
EnableSyslog=false
[Channels]
rdpdr=false
rdpsnd=true
drdynvc=true
cliprdr=false
rail=false
xrdpvr=false
[replica]
name=Isolated lifecycle check
lib=libvnc.so
ip=127.0.0.1
port={vnc_port}
username=na
password=ask
chansrvport=DISPLAY({display},{os.getuid()})
'''
        (root / 'xrdp.ini').write_text(config)
        server = start([vnc_binary, ':' + str(display), '-geometry', '640x480', '-depth', '24',
            '-localhost', '-rfbport', str(vnc_port), '-nolisten', 'tcp',
            '-PasswordFile', str(credential), '-SecurityTypes', 'VncAuth', '-auth', str(auth)], 'vnc')
        start(['Xvfb', ':' + str(client_display), '-screen', '0', '800x600x24',
               '-nolisten', 'tcp', '-auth', str(auth)], 'client-display', client_env)
        time.sleep(.5)
        # Delay the audio core deliberately. The same startup implementation must wait.
        gate_source = ('import importlib.util,sys;'
            's=importlib.util.spec_from_file_location("r",sys.argv[1]);'
            'm=importlib.util.module_from_spec(s);s.loader.exec_module(m);'
            'sys.exit(0 if m.prepare_runtime(__import__("os").getuid(),[int(sys.argv[2])],'
            '__import__("pathlib").Path(sys.argv[3]),20) else 1)')
        gate = start([sys.executable, '-c', gate_source, str(Path(__file__).with_name('linux-workspace-audio-router.py')),
                      str(display), str(runtime)], 'startup-gate')
        time.sleep(.4)
        report['gate_waits_for_delayed_audio'] = gate.poll() is None
        core = start(['pipewire'], 'pipewire')
        wait_socket(runtime / 'pipewire-0', core)
        pulse = start(['pipewire-pulse'], 'pulse')
        wait_socket(runtime / 'pulse/native', pulse)
        # Reproduce the systemd socket unit's normal boot-created child mode.
        (runtime / 'pulse').chmod(0o755)
        start(['dbus-run-session', '--', 'wireplumber', '--profile', 'policy'], 'policy')
        time.sleep(.7)
        start(['/usr/sbin/xrdp-chansrv'], 'channels')
        start(['pw-cli', '-m', '-d', 'load-module', 'libpipewire-module-xrdp',
            f'sink.stream.props={{ node.always-process=true node.pause-on-idle=false node.name=pfremote-speaker-{display} priority.session=0 }}',
            f'source.stream.props={{ node.name=pfremote-mic-{display} priority.session=100 }}'], 'adapter')
        gate.wait(timeout=22)
        report['gate_ready_before_app'] = gate.returncode == 0
        if gate.returncode:
            for command, name in [(['pw-dump'], 'audio-graph.json')]:
                (root / name).write_bytes(run(command, env=env))
            raise RuntimeError('startup gate did not become ready')
        report['boot_source_directory_mode'] = oct((runtime / 'pulse').stat().st_mode & 0o777)
        wm = start(['dbus-run-session', '--', 'xfwm4', '--compositor=off'], 'window-manager')
        start([sys.executable, str(Path(__file__).with_name('linux-workspace-input-guard.py')),
               '--display', str(display)], 'input-guard')
        start(['xrdp', '-n', '-c', str(root / 'xrdp.ini')], 'rdp')
        time.sleep(.5)
        os.environ['XAUTHORITY'] = str(auth)
        window = input_test.TestWindow(display)
        deadline = time.monotonic() + 10
        while not window.x.property(window.window, 'WM_STATE'):
            if wm.poll() is not None or time.monotonic() >= deadline:
                raise RuntimeError('isolated window manager is not ready')
            time.sleep(.05)
        window.activate()
        report['app_initial_keyboard'] = window.type_probe() == 2
        client_x = input_test.guard.X11(client_display)
        for name in ['pfremote-test-speaker', 'pfremote-test-input']:
            start(['pw-cli', '-m', 'create-node', 'adapter',
                f'{{ factory.name=support.null-audio-sink node.name={name} media.class=Audio/Sink audio.position=[FL FR] priority.session=-2000 object.linger=false }}'], name, client_env)
        time.sleep(.4)
        graph = json.loads(run(['pw-dump'], env=env))
        report['physical_audio_devices'] = sum(x['type'] == 'PipeWire:Interface:Device' for x in graph)
        report['audio_core_isolated'] = report['physical_audio_devices'] == 0
        check = run([sys.executable, str(Path(__file__).with_name('verify-linux-workspace-audio.py')),
                     '--display', str(display), '--runtime-root', str(runtime)], env=env)
        report['ordinary_app_audio_ready'] = json.loads(check)['passed']
        for attempt in range(2 if generation == 1 else 1):
            result = {'attempt': attempt + 1}
            client = start(['xfreerdp', '/v:127.0.0.1:' + str(rdp_port), '/u:na', '/d:replica',
                '/from-stdin:force', '/sec:tls', '/cert:fingerprint:sha256:' + fingerprint,
                '/size:640x480', '/title:PF Remote isolated RDP',
                '/sound:sys:pulse,dev:pfremote-test-speaker,latency:200',
                '/microphone:sys:pulse,dev:pfremote-test-input.monitor', '-clipboard', '/log-level:ERROR'],
                'client', client_env, stdin=subprocess.PIPE)
            client.stdin.write(secret + b'\n')
            client.stdin.flush()
            time.sleep(2)
            result['client_alive'] = client.poll() is None
            win = native_client_window(client_x)
            client_x.x.XSetInputFocus(client_x.d, win, 1, 0)
            client_x.x.XSync(client_x.d, False)
            window.key_count()
            for symbol in [ord('a'), ord('1')]:
                code = window.x.x.XKeysymToKeycode(client_x.d, symbol)
                window.xt.XTestFakeKeyEvent(client_x.d, code, True, 0)
                window.xt.XTestFakeKeyEvent(client_x.d, code, False, 0)
            client_x.x.XSync(client_x.d, False)
            time.sleep(.25)
            result['rdp_keyboard'] = window.key_count() == 2
            # The real native client click must recover the isolated fault,
            # including the VNC backend's XI2 event path. Never click a live X.
            window.break_focus()
            result['focus_fault_reproduced'] = input_test.guard.eligible(window.x.snapshot())
            translate = window.x.x.XTranslateCoordinates
            translate.argtypes = [C.c_void_p, C.c_ulong, C.c_ulong, C.c_int, C.c_int,
                                  C.POINTER(C.c_int), C.POINTER(C.c_int), C.POINTER(C.c_ulong)]
            px, py, child = C.c_int(), C.c_int(), C.c_ulong()
            translate(window.x.d, window.window, window.x.root, 150, 120,
                      C.byref(px), C.byref(py), C.byref(child))
            window.x.x.XWarpPointer(client_x.d, 0, win, 0, 0, 0, 0, px.value, py.value)
            window.xt.XTestFakeButtonEvent(client_x.d, 1, True, 0)
            window.xt.XTestFakeButtonEvent(client_x.d, 1, False, 0)
            client_x.x.XSync(client_x.d, False)
            time.sleep(.4)
            window.key_count()
            for symbol in [ord('a'), ord('1')]:
                code = window.x.x.XKeysymToKeycode(client_x.d, symbol)
                window.xt.XTestFakeKeyEvent(client_x.d, code, True, 0)
                window.xt.XTestFakeKeyEvent(client_x.d, code, False, 0)
            client_x.x.XSync(client_x.d, False)
            time.sleep(.25)
            result['remote_click_recovers_keyboard'] = window.key_count() == 2
            raw = ['--raw', '--rate=44100', '--channels=2', '--format=s16']
            for direction, source, dest, monitor in [
                ('playback', f'pfremote-speaker-{display}', 'pfremote-test-speaker', True),
                ('microphone', 'pfremote-test-input', f'pfremote-mic-{display}', False)]:
                capture_file = root / (direction + '.raw')
                with capture_file.open('wb') as capture:
                    args = ['pw-cat', '--record', '--target', dest, *raw]
                    if monitor:
                        args += ['-P', 'stream.capture.sink=true']
                    recorder = subprocess.Popen(args + ['-'], env=client_env, stdout=capture,
                        stderr=subprocess.DEVNULL, start_new_session=True)
                    children.append(recorder)
                    time.sleep(.3)
                    playback = subprocess.Popen(['pw-cat', '--playback', '--target', source, *raw, '-'],
                        env=client_env, stdin=subprocess.PIPE, stdout=subprocess.DEVNULL,
                        stderr=subprocess.DEVNULL, start_new_session=True)
                    children.append(playback)
                    playback.communicate(tone(), timeout=12)
                    time.sleep(.3)
                    stop(recorder)
                result[direction] = measure(capture_file.read_bytes())
            stop(client)
            time.sleep(.3)
            result['desktop_and_app_retained'] = server.poll() is None and window.x.focus() == window.window
            result['passed'] = all(result.get(key) for key in ['client_alive', 'rdp_keyboard',
                'focus_fault_reproduced', 'remote_click_recovers_keyboard', 'desktop_and_app_retained']) and all(result[k]['passed'] for k in ['playback', 'microphone'])
            report['connections'].append(result)
            print(json.dumps({'progress': workspace, 'cold_start': generation, 'connection': result}), flush=True)
        report['passed'] = all(report.get(k) for k in ['gate_waits_for_delayed_audio', 'gate_ready_before_app',
            'app_initial_keyboard', 'audio_core_isolated', 'ordinary_app_audio_ready']) and all(r['passed'] for r in report['connections'])
    except Exception as error:
        report['error_type'] = type(error).__name__
        if isinstance(error, subprocess.CalledProcessError):
            report['failed_program'] = Path(error.cmd[0]).name
            report['exit_code'] = error.returncode
        report['passed'] = False
    finally:
        if window:
            window.x.close()
        if client_x:
            client_x.close()
        if previous_auth is None:
            os.environ.pop('XAUTHORITY', None)
        else:
            os.environ['XAUTHORITY'] = previous_auth
        for process in reversed(children):
            stop(process)
        report['children_stopped'] = all(process.poll() is not None for process in children)
        report['passed'] = report.get('passed', False) and report['children_stopped']
    if report['passed']:
        import shutil
        shutil.rmtree(root)
    else:
        report['private_diagnostics'] = str(root)
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--vnc-binary', default='Xvnc')
    parser.add_argument('--workspaces', type=int, nargs='+', default=[2, 3, 4])
    args = parser.parse_args()
    base = Path.home() / '.local/share/pfremote/diagnostics/workspace-reconnect-20261018'
    base.mkdir(mode=0o700, parents=True, exist_ok=True)
    results = []
    for workspace in args.workspaces:
        for generation in [1, 2]:
            result = replica(workspace, generation, args.vnc_binary, base)
            results.append(result)
            print(json.dumps(result), flush=True)
            if not result['passed']:
                break
        if not results[-1]['passed']:
            break
    summary = {'schema_version': 'pfremote.workspace-lifecycle-check/v1',
        'results': results, 'passed': len(results) == len(args.workspaces) * 2 and all(r['passed'] for r in results)}
    (base / 'lifecycle-results.json').write_text(json.dumps(summary, indent=2))
    print(json.dumps({'completed': True, 'passed': summary['passed']}), flush=True)
    return 0 if summary['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
