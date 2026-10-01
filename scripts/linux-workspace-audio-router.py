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
import subprocess
import time


def process_display(pid, uid):
    process = Path('/proc') / str(pid)
    if process.stat().st_uid != uid:
        return None
    for item in (process / 'environ').read_bytes().split(b'\0'):
        if item.startswith(b'DISPLAY='):
            match = re.fullmatch(rb':([1-9][0-9]?)(?:\.[0-9]+)?', item[8:])
            return int(match[1]) if match else None
    return None


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
    args = parser.parse_args()
    if os.getuid() == 0 or any(d < 1 or d > 99 for d in args.displays):
        parser.error('run as the desktop owner with declared displays 1..99')
    applied = {}
    while True:
        try:
            graph = json.loads(subprocess.check_output(['pw-dump'], timeout=5))
            wanted = decisions(graph, os.getuid(), set(args.displays))
            live = {serial for _, serial, _ in wanted}
            applied = {serial: target for serial, target in applied.items() if serial in live}
            for node_id, serial, target in wanted:
                if applied.get(serial) == target:
                    continue
                # Confirm the current object serial before using its ephemeral ID.
                fresh = json.loads(subprocess.check_output(['pw-dump'], timeout=5))
                if (node_id, serial, target) not in decisions(fresh, os.getuid(), set(args.displays)):
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
