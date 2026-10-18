"""Exercise focus recovery only on a disposable, cookie-protected X display."""
import ctypes as C
import importlib.util
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time

spec = importlib.util.spec_from_file_location('input_guard', Path(__file__).with_name('linux-workspace-input-guard.py'))
guard = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = guard
spec.loader.exec_module(guard)


class MessageData(C.Union):
    _fields_ = [('b', C.c_char * 20), ('s', C.c_short * 10), ('l', C.c_long * 5)]


class Message(C.Structure):
    _fields_ = [('type', C.c_int), ('serial', C.c_ulong), ('send_event', C.c_int),
                ('display', C.c_void_p), ('window', C.c_ulong), ('message_type', C.c_ulong),
                ('format', C.c_int), ('data', MessageData)]


class TestWindow:
    def __init__(self, display):
        self.x = guard.X11(display)
        x, P, W, I = self.x.x, C.c_void_p, C.c_ulong, C.c_int
        for name, args, result in [
            ('XCreateSimpleWindow', [P, W, I, I, C.c_uint, C.c_uint, C.c_uint, W, W], W),
            ('XSelectInput', [P, W, C.c_long], I), ('XMapWindow', [P, W], I),
            ('XChangeProperty', [P, W, W, W, I, I, C.c_void_p, I], I),
            ('XDeleteProperty', [P, W, W], I), ('XStoreName', [P, W, C.c_char_p], I),
            ('XSendEvent', [P, W, I, C.c_long, C.c_void_p], I),
            ('XWarpPointer', [P, W, W, I, I, C.c_uint, C.c_uint, I, I], I),
            ('XKeysymToKeycode', [P, W], C.c_ubyte), ('XDestroyWindow', [P, W], I)]:
            fn = getattr(x, name)
            fn.argtypes, fn.restype = args, result
        self.window = x.XCreateSimpleWindow(self.x.d, self.x.root, 70, 70, 420, 280, 0, 0, 0xffffff)
        self.set('_NET_WM_PID', 'CARDINAL', [os.getpid()])
        self.set('_NET_WM_WINDOW_TYPE', 'ATOM', [self.x.atom('_NET_WM_WINDOW_TYPE_NORMAL')])
        hints = guard.Hints(flags=1, input=1)
        x.XSetWMHints(self.x.d, self.window, C.byref(hints))
        x.XSelectInput(self.x.d, self.window, 1 | 4 | 8 | (1 << 21))
        x.XStoreName(self.x.d, self.window, b'PF Remote isolated input check')
        x.XMapWindow(self.x.d, self.window)
        x.XSync(self.x.d, False)
        self.xt = C.CDLL('libXtst.so.6')
        self.xt.XTestFakeButtonEvent.argtypes = [P, C.c_uint, I, W]
        self.xt.XTestFakeKeyEvent.argtypes = [P, C.c_uint, I, W]

    def set(self, name, kind, values, window=None):
        data = (C.c_ulong * len(values))(*values)
        self.x.x.XChangeProperty(self.x.d, self.window if window is None else window,
            self.x.atom(name), self.x.atom(kind), 32, 0, data, len(values))

    def activate(self):
        event = Message(type=33, display=self.x.d, window=self.window,
            message_type=self.x.atom('_NET_ACTIVE_WINDOW'), format=32)
        event.data.l[0] = 2
        self.x.x.XSendEvent(self.x.d, self.x.root, False, (1 << 19) | (1 << 20), C.byref(event))
        self.x.x.XSync(self.x.d, False)
        time.sleep(.2)

    def point(self):
        self.x.x.XWarpPointer(self.x.d, 0, self.window, 0, 0, 0, 0, 150, 120)
        self.x.x.XSync(self.x.d, False)

    def click(self):
        self.point()
        self.xt.XTestFakeButtonEvent(self.x.d, 1, True, 0)
        self.xt.XTestFakeButtonEvent(self.x.d, 1, False, 0)
        self.x.x.XSync(self.x.d, False)

    def key_count(self):
        count = 0
        while self.x.x.XPending(self.x.d):
            event = guard.Event()
            self.x.x.XNextEvent(self.x.d, C.byref(event))
            count += event.type == 2
        return count

    def type_probe(self):
        self.key_count()
        for symbol in [ord('a'), ord('1')]:
            code = self.x.x.XKeysymToKeycode(self.x.d, symbol)
            self.xt.XTestFakeKeyEvent(self.x.d, code, True, 0)
            self.xt.XTestFakeKeyEvent(self.x.d, code, False, 0)
        self.x.x.XSync(self.x.d, False)
        time.sleep(.1)
        return self.key_count()

    def break_focus(self, missing_hints=True):
        if missing_hints:
            # Reproduce XFWM's cached no-input flag when an app later removes
            # WM_HINTS, rather than pretending a missing hint alone is broken.
            hints = guard.Hints(flags=1, input=0)
            self.x.x.XSetWMHints(self.x.d, self.window, C.byref(hints))
            self.x.x.XSync(self.x.d, False)
            time.sleep(.1)
            self.x.x.XDeleteProperty(self.x.d, self.window, self.x.atom('WM_HINTS'))
        fallback = self.x.property(self.x.root, '_NET_SUPPORTING_WM_CHECK')[0]
        self.x.x.XSetInputFocus(self.x.d, fallback, 1, 0)
        self.x.x.XSync(self.x.d, False)
        time.sleep(.15)
        self.point()


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


def main():
    base = Path.home() / '.local/share/pfremote/diagnostics'
    base.mkdir(mode=0o700, parents=True, exist_ok=True)
    report, children = {}, []
    with tempfile.TemporaryDirectory(prefix='isolated-focus-', dir=base) as tmp:
        root = Path(tmp)
        display = next(d for d in range(80, 100) if not Path(f'/tmp/.X11-unix/X{d}').exists()
                       and not Path(f'/tmp/.X{d}-lock').exists())
        auth = root / 'Xauthority'
        auth.touch(mode=0o600)
        subprocess.run(['xauth', '-f', str(auth), 'add', ':' + str(display),
                        'MIT-MAGIC-COOKIE-1', os.urandom(16).hex()], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        env = dict(os.environ, DISPLAY=':' + str(display), XAUTHORITY=str(auth),
                   XDG_CONFIG_HOME=str(root / 'config'), XDG_CACHE_HOME=str(root / 'cache'),
                   GIO_USE_VFS='local', NO_AT_BRIDGE='1')
        def start(args, name):
            process = subprocess.Popen(args, env=env, stdout=(root / (name + '.log')).open('wb'),
                                       stderr=subprocess.STDOUT, start_new_session=True)
            children.append(process)
            return process
        previous_auth = os.environ.get('XAUTHORITY')
        try:
            start(['Xvfb', ':' + str(display), '-screen', '0', '800x600x24',
                   '-nolisten', 'tcp', '-auth', str(auth)], 'xserver')
            time.sleep(.4)
            start(['dbus-run-session', '--', 'xfwm4', '--compositor=off'], 'wm')
            time.sleep(.6)
            os.environ['XAUTHORITY'] = str(auth)
            window = TestWindow(display)
            window.activate()
            report['normal_input'] = window.type_probe() == 2
            window.break_focus()
            report['known_fault_reproduced'] = guard.eligible(window.x.snapshot())
            if not report['known_fault_reproduced']:
                report['fault_state'] = window.x.snapshot().__dict__
            window.click()
            time.sleep(.25)
            report['native_click_leaves_fault'] = window.type_probe() == 0
            daemon = start([sys.executable, str(Path(__file__).with_name('linux-workspace-input-guard.py')),
                            '--display', str(display)], 'guard')
            time.sleep(.3)
            report['no_unsolicited_focus_change'] = guard.eligible(window.x.snapshot())
            window.click()
            time.sleep(.35)
            report['click_recovers_input'] = window.type_probe() == 2
            for _ in range(3):
                window.break_focus()
                window.click()
                time.sleep(.3)
                report['repeated_recovery'] = window.type_probe() == 2
                if not report['repeated_recovery']:
                    break
            hints = guard.Hints(flags=1, input=0)
            window.x.x.XSetWMHints(window.x.d, window.window, C.byref(hints))
            window.break_focus(missing_hints=False)
            window.click()
            time.sleep(.3)
            report['explicit_no_input_preserved'] = window.x.focus() != window.window
            report['guard_alive'] = daemon.poll() is None
            window.x.close()
        except Exception as error:
            report['error_type'] = type(error).__name__
        finally:
            if previous_auth is None:
                os.environ.pop('XAUTHORITY', None)
            else:
                os.environ['XAUTHORITY'] = previous_auth
            for process in reversed(children):
                stop(process)
            report['children_stopped'] = all(process.poll() is not None for process in children)
        report['passed'] = all(report.get(k) is True for k in [
            'normal_input', 'known_fault_reproduced', 'native_click_leaves_fault',
            'no_unsolicited_focus_change', 'click_recovers_input', 'repeated_recovery',
            'explicit_no_input_preserved', 'guard_alive', 'children_stopped'])
    print(json.dumps(report))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
