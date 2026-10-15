"""Route this user's desktop app streams to their declared shared workspace.

Uses WirePlumber's per-stream metadata, never changes global audio defaults,
captures samples, or changes the host network. Explicit non-PF audio choices
remain with the application. Run as the desktop owner, not root.
"""
import argparse
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import time


def process_display(pid, uid, window_displays=None, proc_root=Path('/proc')):
    """Chromium children may clear DISPLAY; follow only same-owner parents."""
    seen = set()
    for _ in range(24):
        if pid < 1 or pid in seen:
            return None
        seen.add(pid)
        process = proc_root / str(pid)
        if process.stat().st_uid != uid:
            return None
        windows = (window_displays or {}).get(pid, set())
        if windows:
            # A process with windows on several workspaces is ambiguous.
            return next(iter(windows)) if len(windows) == 1 else None
        for item in (process / 'environ').read_bytes().split(b'\0'):
            if item.startswith(b'DISPLAY='):
                match = re.fullmatch(rb':([1-9][0-9]?)(?:\.[0-9]+)?', item[8:])
                return int(match[1]) if match else None
        status = (process / 'status').read_text()
        parent = re.search(r'^PPid:\s*(\d+)$', status, re.M)
        if not parent:
            return None
        pid = int(parent[1])
    return None


def window_processes(displays, uid, proc_root=Path('/proc')):
    """Read PID properties only, never titles, pixels, input or focus."""
    result = {}
    deadline = time.monotonic() + 3
    for display in sorted(displays):
        try:
            args = ['xprop', '-display', ':' + str(display)]
            raw = subprocess.check_output(args + ['-root', '_NET_CLIENT_LIST'],
                                          stderr=subprocess.DEVNULL, timeout=.5)
            for window in re.findall(rb'0x[0-9a-fA-F]+', raw)[:128]:
                if time.monotonic() >= deadline:
                    return result
                raw = subprocess.check_output(args + ['-id', window.decode(), '_NET_WM_PID'],
                                              stderr=subprocess.DEVNULL, timeout=.5)
                match = re.search(rb'=\s*(\d+)\s*$', raw)
                if match:
                    pid = int(match[1])
                    if (proc_root / str(pid)).stat().st_uid == uid:
                        result.setdefault(pid, set()).add(display)
        except (OSError, subprocess.SubprocessError):
            continue
    return result


class DisplayResolver:
    def __init__(self, uid, displays):
        self.uid, self.displays = uid, displays
        self.windows, self.refreshed = {}, -float('inf')

    def __call__(self, pid, uid):
        direct = process_display(pid, uid)
        if direct is not None:
            return direct
        if time.monotonic() - self.refreshed >= 5:
            self.windows = window_processes(self.displays, self.uid)
            self.refreshed = time.monotonic()
        return process_display(pid, uid, self.windows)


def protected_directory(path, uid):
    st = path.lstat()
    return stat.S_ISDIR(st.st_mode) and st.st_uid == uid and st.st_mode & 0o077 == 0


def ensure_runtime_audio(uid, displays, runtime=None):
    """Share only audio sockets with existing private per-workspace runtimes.

    XFCE keeps a separate XDG_RUNTIME_DIR per VNC desktop for session isolation.
    Do not replace it or its bus. Missing audio links are repaired after login;
    existing servers, files and conflicting links are always left untouched.
    """
    runtime = Path(runtime) if runtime is not None else Path('/run/user') / str(uid)
    created = []
    if not protected_directory(runtime, uid):
        return created
    for display in sorted(displays):
        workspace = runtime / ('vnc-' + str(display))
        try:
            if not protected_directory(workspace, uid):
                continue
            for relative in ('pulse/native', 'pipewire-0'):
                target, link = runtime / relative, workspace / relative
                if relative.startswith('pulse/') and not protected_directory(target.parent, uid):
                    continue
                st = target.lstat()
                if not stat.S_ISSOCK(st.st_mode) or st.st_uid != uid:
                    continue
                if relative.startswith('pulse/'):
                    link.parent.mkdir(mode=0o700, exist_ok=True)
                    if not protected_directory(link.parent, uid):
                        continue
                if not link.exists() and not link.is_symlink():
                    link.symlink_to(target)
                    created.append(link)
        except OSError:
            # Missing/recreated desktop runtimes are normal; retry next pass.
            continue
    return created


def remove_runtime_audio(uid, displays, runtime=None):
    """Rollback only exact links to our same-owner audio endpoints."""
    runtime = Path(runtime) if runtime is not None else Path('/run/user') / str(uid)
    if not protected_directory(runtime, uid):
        return
    for display in displays:
        workspace = runtime / ('vnc-' + str(display))
        try:
            if not protected_directory(workspace, uid):
                continue
            for relative in ('pulse/native', 'pipewire-0'):
                link = workspace / relative
                if not protected_directory(link.parent, uid):
                    continue
                if link.is_symlink() and link.readlink() == runtime / relative:
                    link.unlink()
        except OSError:
            continue


def decisions(graph, uid, displays, display_reader=process_display):
    clients = {x['id']: x.get('info', {}).get('props', {}) for x in graph
               if x.get('type') == 'PipeWire:Interface:Client'}
    nodes = [x for x in graph if x.get('type') == 'PipeWire:Interface:Node']
    destinations = {x.get('info', {}).get('props', {}).get('node.name') for x in nodes}
    node_names = {}
    for node in nodes:
        props = node.get('info', {}).get('props', {})
        for key in (node['id'], props.get('object.serial')):
            if key is not None:
                node_names[str(key)] = props.get('node.name', '')
    choices = {}
    for metadata in graph:
        if metadata.get('type') != 'PipeWire:Interface:Metadata' or metadata.get('props', {}).get('metadata.name') != 'default':
            continue
        for choice in metadata.get('metadata', []):
            if choice.get('key') in ('target.object', 'target.node'):
                choices[choice.get('subject')] = str(choice.get('value', ''))
    result = []
    for node in nodes:
        props = node.get('info', {}).get('props', {})
        direction = {'Stream/Output/Audio': 'speaker', 'Stream/Input/Audio': 'mic'}.get(props.get('media.class'))
        if not direction:
            continue
        explicit = choices.get(node['id'], str(props.get('target.object', '')))
        explicit = node_names.get(explicit, explicit)
        if explicit not in ('', '0', 'default') and not explicit.startswith('pfremote-'):
            continue
        pid = props.get('application.process.id') or clients.get(props.get('client.id'), {}).get('application.process.id')
        try:
            display = display_reader(int(pid), uid)
        except (TypeError, ValueError, OSError):
            continue
        target = f'pfremote-{direction}-{display}'
        if display in displays and target in destinations and props.get('object.serial') is not None:
            result.append((node['id'], props['object.serial'], target))
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--displays', nargs='+', type=int, default=[2, 3, 4])
    parser.add_argument('--remove-runtime-links', action='store_true')
    args = parser.parse_args()
    if os.getuid() == 0 or any(d < 1 or d > 99 for d in args.displays):
        parser.error('run as the desktop owner with declared displays 1..99')
    if args.remove_runtime_links:
        remove_runtime_audio(os.getuid(), args.displays)
        return
    resolver = DisplayResolver(os.getuid(), set(args.displays))
    applied = {}
    runtime_checked = -float('inf')
    while True:
        try:
            if time.monotonic() - runtime_checked >= 5:
                ensure_runtime_audio(os.getuid(), args.displays)
                runtime_checked = time.monotonic()
            graph = json.loads(subprocess.check_output(['pw-dump'], timeout=5))
            wanted = decisions(graph, os.getuid(), set(args.displays), resolver)
            live = {serial for _, serial, _ in wanted}
            applied = {serial: target for serial, target in applied.items() if serial in live}
            for node_id, serial, target in wanted:
                if applied.get(serial) == target:
                    continue
                # Confirm the current object serial before using its ephemeral ID.
                fresh = json.loads(subprocess.check_output(['pw-dump'], timeout=5))
                if (node_id, serial, target) not in decisions(fresh, os.getuid(), set(args.displays), resolver):
                    continue
                subprocess.run(['pw-metadata', '-n', 'default', str(node_id), 'target.object', target, 'Spa:String'],
                               check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=5)
                applied[serial] = target
        except (OSError, ValueError, subprocess.SubprocessError):
            # Retry node/core restart without emitting process metadata or paths.
            applied.clear()
        time.sleep(.5)


if __name__ == '__main__':
    main()
