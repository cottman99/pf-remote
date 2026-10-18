"""Check ordinary workspace audio discovery and routing without audio samples.

Creates paused PulseAudio streams in the same isolated runtime as desktop apps.
Does not open a remote desktop, read a microphone, play a tone or change defaults.
"""
import argparse
import ctypes as c
import ctypes.util
import json
import os
from pathlib import Path
import subprocess
import sys
import time


def check(display, runtime_root=None):
    uid = os.getuid()
    runtime_root = Path(runtime_root) if runtime_root is not None else Path('/run/user') / str(uid)
    os.environ['DISPLAY'] = ':' + str(display)
    os.environ['XDG_RUNTIME_DIR'] = str(runtime_root / ('vnc-' + str(display)))
    pulse = c.CDLL(c.util.find_library('pulse'))
    functions = [
        ('pa_mainloop_new', c.c_void_p, []),
        ('pa_mainloop_get_api', c.c_void_p, [c.c_void_p]),
        ('pa_mainloop_iterate', c.c_int, [c.c_void_p, c.c_int, c.c_void_p]),
        ('pa_mainloop_free', None, [c.c_void_p]),
        ('pa_context_new', c.c_void_p, [c.c_void_p, c.c_char_p]),
        ('pa_context_connect', c.c_int, [c.c_void_p, c.c_char_p, c.c_int, c.c_void_p]),
        ('pa_context_get_state', c.c_int, [c.c_void_p]),
        ('pa_context_errno', c.c_int, [c.c_void_p]),
        ('pa_context_disconnect', None, [c.c_void_p]),
        ('pa_context_unref', None, [c.c_void_p]),
        ('pa_stream_new', c.c_void_p, [c.c_void_p, c.c_char_p, c.c_void_p, c.c_void_p]),
        ('pa_stream_connect_playback', c.c_int, [c.c_void_p, c.c_char_p, c.c_void_p, c.c_int, c.c_void_p, c.c_void_p]),
        ('pa_stream_connect_record', c.c_int, [c.c_void_p, c.c_char_p, c.c_void_p, c.c_int]),
        ('pa_stream_get_state', c.c_int, [c.c_void_p]),
        ('pa_stream_disconnect', c.c_int, [c.c_void_p]),
        ('pa_stream_unref', None, [c.c_void_p]),
    ]
    for name, result, args in functions:
        function = getattr(pulse, name)
        function.restype, function.argtypes = result, args

    class SampleSpec(c.Structure):
        _fields_ = [('format', c.c_int), ('rate', c.c_uint32), ('channels', c.c_uint8)]

    class ServerInfo(c.Structure):
        # Only the ABI prefix through default_source_name is needed.
        _fields_ = [('user_name', c.c_char_p), ('host_name', c.c_char_p),
                    ('server_version', c.c_char_p), ('server_name', c.c_char_p),
                    ('sample_spec', SampleSpec), ('default_sink_name', c.c_char_p),
                    ('default_source_name', c.c_char_p)]

    loop = pulse.pa_mainloop_new()
    context = pulse.pa_context_new(pulse.pa_mainloop_get_api(loop), b'PF Remote paused audio check')
    streams = []
    report = {'schema_version': 'pfremote.workspace-audio-check/v1', 'display': display,
              'pulse_connected': False, 'playback_route': False, 'microphone_route': False,
              'default_source_is_microphone': False,
              'samples_read_or_played': 0, 'stage': 'connect'}

    def iterate_until(condition, seconds):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            pulse.pa_mainloop_iterate(loop, 0, None)
            if condition():
                return True
            time.sleep(.01)
        return False

    try:
        # NOAUTOSPAWN: do not create a different audio server to hide a failure.
        endpoint = ('unix:' + str(runtime_root / f'vnc-{display}/pulse/native')).encode()
        if pulse.pa_context_connect(context, endpoint, 1, None) < 0:
            return report
        report['pulse_connected'] = iterate_until(lambda: pulse.pa_context_get_state(context) == 4, 3)
        if not report['pulse_connected']:
            return report
        report['stage'] = 'default-input'
        received = []
        callback_type = c.CFUNCTYPE(None, c.c_void_p, c.POINTER(ServerInfo), c.c_void_p)

        @callback_type
        def server_info(_context, info, _data):
            source = info.contents.default_source_name if info else None
            received.append(source)
            report['default_source_is_microphone'] = bool(source and not source.endswith(b'.monitor'))

        pulse.pa_context_get_server_info.argtypes = [c.c_void_p, callback_type, c.c_void_p]
        pulse.pa_context_get_server_info.restype = c.c_void_p
        pulse.pa_operation_unref.argtypes = [c.c_void_p]
        pulse.pa_operation_unref.restype = None
        operation = pulse.pa_context_get_server_info(context, server_info, None)
        if not operation:
            return report
        try:
            if not iterate_until(lambda: bool(received), 3) or not report['default_source_is_microphone']:
                return report
        finally:
            pulse.pa_operation_unref(operation)
        spec = SampleSpec(3, 44100, 2)  # PA_SAMPLE_S16LE
        for direction in ['playback', 'microphone']:
            report['stage'] = direction + '-stream'
            stream = pulse.pa_stream_new(context, ('PF Remote paused ' + direction).encode(), c.byref(spec), None)
            if not stream:
                return report
            streams.append(stream)
            # START_CORKED: a real discoverable app stream, no capture/playback.
            if direction == 'playback':
                result = pulse.pa_stream_connect_playback(stream, None, None, 1, None, None)
            else:
                result = pulse.pa_stream_connect_record(stream, None, None, 1)
            if result < 0 or not iterate_until(lambda: pulse.pa_stream_get_state(stream) == 2, 3):
                report['stream_state'] = pulse.pa_stream_get_state(stream)
                report['pulse_error'] = pulse.pa_context_errno(context)
                return report
        report['stage'] = 'routing'
        deadline = time.monotonic() + 6
        while time.monotonic() < deadline:
            iterate_until(lambda: False, .2)
            graph = json.loads(subprocess.check_output(['pw-dump'], timeout=3))
            nodes = {x['id']: x.get('info', {}).get('props', {}) for x in graph
                     if x['type'] == 'PipeWire:Interface:Node'}
            for node_id, props in nodes.items():
                if str(props.get('application.process.id')) != str(os.getpid()):
                    continue
                direction = {'Stream/Output/Audio': 'playback', 'Stream/Input/Audio': 'microphone'}.get(props.get('media.class'))
                if not direction:
                    continue
                for link in (x.get('info', {}) for x in graph if x['type'] == 'PipeWire:Interface:Link'):
                    app_key, device_key = ('output-node-id', 'input-node-id') if direction == 'playback' else ('input-node-id', 'output-node-id')
                    expected = 'pfremote-' + ('speaker' if direction == 'playback' else 'mic') + '-' + str(display)
                    if link.get(app_key) == node_id and nodes.get(link.get(device_key), {}).get('node.name') == expected:
                        report[direction + '_route'] = True
            if report['playback_route'] and report['microphone_route']:
                break
        return report
    finally:
        for stream in streams:
            pulse.pa_stream_disconnect(stream)
            pulse.pa_stream_unref(stream)
        pulse.pa_context_disconnect(context)
        pulse.pa_context_unref(context)
        pulse.pa_mainloop_free(loop)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--display', type=int, required=True, choices=range(1, 100))
    parser.add_argument('--runtime-root', type=Path, default=Path('/run/user') / str(os.getuid()))
    args = parser.parse_args()
    runtime = str(args.runtime_root / ('vnc-' + str(args.display)))
    if os.environ.get('DISPLAY') != ':' + str(args.display) or os.environ.get('XDG_RUNTIME_DIR') != runtime:
        env = os.environ.copy()
        env.update(DISPLAY=':' + str(args.display), XDG_RUNTIME_DIR=runtime, PIPEWIRE_RUNTIME_DIR=runtime)
        # /proc/PID/environ reflects exec-time variables, as for real desktop apps.
        os.execve(sys.executable, [sys.executable, __file__, *sys.argv[1:]], env)
    result = check(args.display, args.runtime_root)
    result['passed'] = all(result[k] for k in ['pulse_connected', 'default_source_is_microphone',
                                             'playback_route', 'microphone_route'])
    print(json.dumps(result))
    raise SystemExit(0 if result['passed'] else 1)
