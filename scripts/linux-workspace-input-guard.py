"""Recover a lost XFWM input focus only after the owner's left-click.

Listen only for XI2 button releases, never keys, audio, titles or pixels. A
normal focus, popup, explicit no-input window or changed user selection wins.
Run one instance per owned local X display; no application is restarted.
"""
import argparse
import ctypes as C
import ctypes.util
from dataclasses import dataclass
import os
from pathlib import Path
import select
import stat
import time


@dataclass(frozen=True)
class Snapshot:
    active: int = 0
    focus: int = 0
    fallback: int = 0
    process: tuple = ()
    window_manager: tuple = ()
    pointer_inside: bool = False
    normal: bool = False
    accepts_input: bool = False
    hints_missing: bool = False
    buttons_down: bool = False
    advertised: int = 0


def eligible(s):
    return bool(s.active and s.fallback and s.active != s.fallback and
                s.advertised in (0, s.active) and
                s.focus == s.fallback and s.process and s.window_manager and
                s.pointer_inside and s.normal and s.accepts_input and not s.buttons_down)


class ClickRecovery:
    def __init__(self):
        self.pending = None

    def click(self, snapshot, timestamp, now):
        self.pending = (snapshot, timestamp, now + .12) if eligible(snapshot) else None

    def due(self, current, now):
        if self.pending is None:
            return None
        previous, timestamp, deadline = self.pending
        if not eligible(current) or current != previous or now > deadline + 1:
            self.pending = None
            return None
        if now < deadline:
            return None
        self.pending = None
        return timestamp


class Cookie(C.Structure):
    _fields_ = [('type', C.c_int), ('serial', C.c_ulong), ('send_event', C.c_int),
                ('display', C.c_void_p), ('extension', C.c_int), ('evtype', C.c_int),
                ('cookie', C.c_uint), ('data', C.c_void_p)]


class Event(C.Union):
    _fields_ = [('type', C.c_int), ('cookie', Cookie), ('padding', C.c_long * 24)]


class RawButton(C.Structure):
    # Only the fixed header is read; no valuator or keyboard data is selected.
    _fields_ = [('type', C.c_int), ('serial', C.c_ulong), ('send_event', C.c_int),
                ('display', C.c_void_p), ('extension', C.c_int), ('evtype', C.c_int),
                ('time', C.c_ulong), ('deviceid', C.c_int), ('sourceid', C.c_int),
                ('detail', C.c_int)]


class EventMask(C.Structure):
    _fields_ = [('deviceid', C.c_int), ('mask_len', C.c_int),
                ('mask', C.POINTER(C.c_ubyte))]


class Attributes(C.Structure):
    _fields_ = [(n, C.c_int) for n in ('x', 'y', 'width', 'height', 'border', 'depth')] + [
        ('visual', C.c_void_p), ('root', C.c_ulong), ('window_class', C.c_int),
        ('bit_gravity', C.c_int), ('win_gravity', C.c_int), ('backing_store', C.c_int),
        ('backing_planes', C.c_ulong), ('backing_pixel', C.c_ulong), ('save_under', C.c_int),
        ('colormap', C.c_ulong), ('map_installed', C.c_int), ('map_state', C.c_int),
        ('all_event_masks', C.c_long), ('your_event_mask', C.c_long),
        ('do_not_propagate_mask', C.c_long), ('override_redirect', C.c_int),
        ('screen', C.c_void_p)]


class Hints(C.Structure):
    _fields_ = [('flags', C.c_long), ('input', C.c_int), ('initial_state', C.c_int),
                ('icon_pixmap', C.c_ulong), ('icon_window', C.c_ulong),
                ('icon_x', C.c_int), ('icon_y', C.c_int), ('icon_mask', C.c_ulong),
                ('window_group', C.c_ulong)]


def process_identity(pid, uid, root=Path('/proc')):
    try:
        p = root / str(pid)
        if pid < 1 or p.stat().st_uid != uid:
            return ()
        return pid, p.joinpath('stat').read_text().rsplit(')', 1)[1].split()[19]
    except (OSError, IndexError):
        return ()


class X11:
    def __init__(self, display):
        self.x = C.CDLL(ctypes.util.find_library('X11'))
        self.xi = C.CDLL(ctypes.util.find_library('Xi'))
        P, W, I = C.c_void_p, C.c_ulong, C.c_int
        signatures = {
            'XOpenDisplay': ([C.c_char_p], P), 'XCloseDisplay': ([P], I),
            'XDefaultRootWindow': ([P], W), 'XConnectionNumber': ([P], I),
            'XInternAtom': ([P, C.c_char_p, I], W), 'XFree': ([P], I),
            'XGetWindowProperty': ([P, W, W, C.c_long, C.c_long, I, W,
                C.POINTER(W), C.POINTER(I), C.POINTER(W), C.POINTER(W), C.POINTER(P)], I),
            'XGetInputFocus': ([P, C.POINTER(W), C.POINTER(I)], I),
            'XSetInputFocus': ([P, W, I, W], I), 'XSync': ([P, I], I),
            'XGetWindowAttributes': ([P, W, C.POINTER(Attributes)], I),
            'XQueryPointer': ([P, W, C.POINTER(W), C.POINTER(W), C.POINTER(I),
                C.POINTER(I), C.POINTER(I), C.POINTER(I), C.POINTER(C.c_uint)], I),
            'XSetWMHints': ([P, W, C.POINTER(Hints)], I),
            'XQueryExtension': ([P, C.c_char_p, C.POINTER(I), C.POINTER(I), C.POINTER(I)], I),
            'XPending': ([P], I), 'XNextEvent': ([P, C.POINTER(Event)], I),
            'XGetEventData': ([P, C.POINTER(Cookie)], I),
            'XFreeEventData': ([P, C.POINTER(Cookie)], None)}
        for name, (args, result) in signatures.items():
            fn = getattr(self.x, name)
            fn.argtypes, fn.restype = args, result
        self.errors = 0
        callback = C.CFUNCTYPE(I, P, P)
        self.error_handler = callback(self._error)
        self.x.XSetErrorHandler.argtypes = [callback]
        self.x.XSetErrorHandler(self.error_handler)
        self.d = self.x.XOpenDisplay((':' + str(display)).encode())
        if not self.d:
            raise OSError('local display unavailable')
        self.root = self.x.XDefaultRootWindow(self.d)
        self.uid, self.atoms = os.getuid(), {}

    def _error(self, *_):
        self.errors += 1
        return 0

    def atom(self, name):
        if name not in self.atoms:
            self.atoms[name] = self.x.XInternAtom(self.d, name.encode(), False)
        return self.atoms[name]

    def property(self, window, name):
        typ, fmt, count, rest, data = C.c_ulong(), C.c_int(), C.c_ulong(), C.c_ulong(), C.c_void_p()
        rc = self.x.XGetWindowProperty(self.d, window, self.atom(name), 0, 256, False,
            0, C.byref(typ), C.byref(fmt), C.byref(count), C.byref(rest), C.byref(data))
        try:
            if rc or rest.value or fmt.value != 32:
                return []
            return list(C.cast(data, C.POINTER(C.c_ulong))[:count.value])
        finally:
            if data:
                self.x.XFree(data)

    def attributes(self, window):
        value = Attributes()
        return value if self.x.XGetWindowAttributes(self.d, window, C.byref(value)) else None

    def focus(self):
        value, revert = C.c_ulong(), C.c_int()
        self.x.XGetInputFocus(self.d, C.byref(value), C.byref(revert))
        return value.value

    def pointer(self, active):
        window, inside, popup = self.root, False, False
        mask = C.c_uint()
        for _ in range(32):
            root, child = C.c_ulong(), C.c_ulong()
            xy = [C.c_int() for _ in range(4)]
            if not self.x.XQueryPointer(self.d, window, C.byref(root), C.byref(child),
                    *[C.byref(v) for v in xy], C.byref(mask)):
                return False, True
            if not child.value:
                break
            window = child.value
            if window == active:
                inside = True
            if inside:
                a = self.attributes(window)
                if a is None or a.override_redirect:
                    popup = True
        return inside and not popup, bool(mask.value & 0x1f00)

    def pointed_client(self):
        """A real click may find XFWM has cleared its active-window property."""
        window = self.root
        for _ in range(32):
            root, child, mask = C.c_ulong(), C.c_ulong(), C.c_uint()
            xy = [C.c_int() for _ in range(4)]
            if not self.x.XQueryPointer(self.d, window, C.byref(root), C.byref(child),
                    *[C.byref(v) for v in xy], C.byref(mask)) or not child.value:
                return 0
            window = child.value
            if (self.property(window, 'WM_STATE') or [0])[0] == 1 and self.property(window, '_NET_WM_PID'):
                return window
        return 0

    def snapshot(self):
        errors = self.errors
        advertised = (self.property(self.root, '_NET_ACTIVE_WINDOW') or [0])[0]
        active = advertised
        fallback = (self.property(self.root, '_NET_SUPPORTING_WM_CHECK') or [0])[0]
        focus = self.focus()
        if not active and fallback and focus == fallback:
            active = self.pointed_client()
        if not active or not fallback or focus != fallback:
            return Snapshot(active=active, focus=focus, fallback=fallback)
        if (self.property(fallback, '_NET_SUPPORTING_WM_CHECK') or [0])[0] != fallback:
            return Snapshot()
        process = process_identity((self.property(active, '_NET_WM_PID') or [0])[0], self.uid)
        wm = process_identity((self.property(fallback, '_NET_WM_PID') or [0])[0], self.uid)
        a = self.attributes(active)
        kind = self.property(active, '_NET_WM_WINDOW_TYPE')
        states = self.property(active, '_NET_WM_STATE')
        normal = bool(a and a.map_state == 2 and not a.override_redirect and
            (not kind or kind == [self.atom('_NET_WM_WINDOW_TYPE_NORMAL')]) and
            self.atom('_NET_WM_STATE_HIDDEN') not in states and
            self.atom('_NET_WM_STATE_MODAL') not in states)
        hints = self.property(active, 'WM_HINTS')
        accepts = not hints or not (hints[0] & 1) or (len(hints) > 1 and hints[1] == 1)
        inside, down = self.pointer(active)
        if self.errors != errors:
            return Snapshot()
        return Snapshot(active, focus, fallback, process, wm, inside, normal, accepts, not hints, down, advertised)

    def restore(self, expected, timestamp):
        # Re-check immediately, and let the X server reject stale click timestamps.
        if self.snapshot() != expected or not eligible(expected):
            return False
        errors = self.errors
        if expected.hints_missing:
            hints = Hints(flags=1, input=1)
            self.x.XSetWMHints(self.d, expected.active, C.byref(hints))
        self.x.XSetInputFocus(self.d, expected.active, 1, timestamp)
        self.x.XSync(self.d, False)
        return self.errors == errors and self.focus() == expected.active

    def subscribe(self):
        opcode, event, error = C.c_int(), C.c_int(), C.c_int()
        if not self.x.XQueryExtension(self.d, b'XInputExtension', C.byref(opcode), C.byref(event), C.byref(error)):
            raise OSError('XInput unavailable')
        self.opcode = opcode.value
        self.xi.XIQueryVersion.argtypes = [C.c_void_p, C.POINTER(C.c_int), C.POINTER(C.c_int)]
        major, minor = C.c_int(2), C.c_int(1)
        if self.xi.XIQueryVersion(self.d, C.byref(major), C.byref(minor)) != 0:
            raise OSError('XInput 2.1 unavailable')
        # XIAllMasterDevices; XI_RawButtonRelease only. No key event subscription.
        data = (C.c_ubyte * 3)(0, 0, 1)
        mask = EventMask(1, 3, data)
        self.xi.XISelectEvents.argtypes = [C.c_void_p, C.c_ulong, C.POINTER(EventMask), C.c_int]
        self.xi.XISelectEvents(self.d, self.root, C.byref(mask), 1)
        self.x.XSync(self.d, False)

    def clicks(self):
        result = []
        while self.x.XPending(self.d):
            event = Event()
            self.x.XNextEvent(self.d, C.byref(event))
            cookie = event.cookie
            if event.type != 35 or cookie.extension != self.opcode or cookie.evtype != 16:
                continue
            if self.x.XGetEventData(self.d, C.byref(cookie)):
                try:
                    raw = C.cast(cookie.data, C.POINTER(RawButton)).contents
                    if raw.detail == 1 and not raw.send_event:
                        result.append(raw.time)
                finally:
                    self.x.XFreeEventData(self.d, C.byref(cookie))
        return result

    def close(self):
        self.x.XCloseDisplay(self.d)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--display', type=int, required=True)
    args = parser.parse_args()
    if os.getuid() == 0 or not 1 <= args.display <= 99:
        parser.error('run as the owner of a declared local display 1..99')
    path = Path('/tmp/.X11-unix') / ('X' + str(args.display))
    socket_stat = path.lstat()
    if socket_stat.st_uid != os.getuid() or not stat.S_ISSOCK(socket_stat.st_mode):
        raise OSError('display socket is not owned by this user')
    x = X11(args.display)
    guard = ClickRecovery()
    try:
        x.subscribe()
        while True:
            for timestamp in x.clicks():
                guard.click(x.snapshot(), timestamp, time.monotonic())
            if guard.pending:
                current = x.snapshot()
                timestamp = guard.due(current, time.monotonic())
                if timestamp is not None:
                    x.restore(current, timestamp)
            select.select([x.x.XConnectionNumber(x.d)], [], [], .05 if guard.pending else 1)
    finally:
        x.close()


if __name__ == '__main__':
    main()
