import importlib.util
import os
from pathlib import Path
import socket
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('router', Path(__file__).with_name('linux-workspace-audio-router.py'))
router = importlib.util.module_from_spec(spec)
spec.loader.exec_module(router)


class RoutingTests(unittest.TestCase):
    def graph(self, media='Stream/Output/Audio', target=None):
        props = {'media.class': media, 'client.id': 10, 'object.serial': 100}
        if target is not None:
            props['target.object'] = target
        return [
            {'id': 10, 'type': 'PipeWire:Interface:Client', 'info': {'props': {'application.process.id': 123}}},
            {'id': 11, 'type': 'PipeWire:Interface:Node', 'info': {'props': props}},
            *[{'id': i, 'type': 'PipeWire:Interface:Node', 'info': {'props': {'node.name': name}}}
              for i, name in enumerate(['pfremote-speaker-2', 'pfremote-speaker-3', 'pfremote-mic-3'], 20)]]

    def test_exact_workspace_in_both_directions(self):
        reader = lambda pid, uid: 3
        self.assertEqual(router.decisions(self.graph(), 1234, {2, 3}, reader), [(11, 100, 'pfremote-speaker-3')])
        self.assertEqual(router.decisions(self.graph('Stream/Input/Audio'), 1234, {2, 3}, reader), [(11, 100, 'pfremote-mic-3')])

    def test_no_sibling_fallback_or_override_of_private_choice(self):
        self.assertEqual(router.decisions(self.graph(), 1234, {2}, lambda *_: 3), [])
        self.assertEqual(router.decisions(self.graph(), 1234, {2, 3, 4}, lambda *_: 4), [])
        self.assertEqual(router.decisions(self.graph(target='personal-headset'), 1234, {3}, lambda *_: 3), [])
        self.assertEqual(router.decisions(self.graph(), 1234, {3}, lambda *_: None), [])
        graph = self.graph()
        graph.append({'type': 'PipeWire:Interface:Metadata', 'props': {'metadata.name': 'default'},
                      'metadata': [{'subject': 11, 'key': 'target.object', 'value': 'personal-headset'}]})
        self.assertEqual(router.decisions(graph, 1234, {3}, lambda *_: 3), [])

    def test_sanitized_browser_child_uses_window_owner_workspace(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for pid, parent, env in [(123, 120, b''), (120, 1, b''), (1, 0, b'')]:
                process = root / str(pid)
                process.mkdir()
                (process / 'environ').write_bytes(env)
                (process / 'status').write_text(f'PPid:\t{parent}\n')
            uid = (root / '123').stat().st_uid
            reader = lambda pid, uid: router.process_display(pid, uid, {120: {3}}, root)
            self.assertEqual(router.process_display(123, uid, proc_root=root), None)
            self.assertEqual(router.decisions(self.graph(), uid, {2, 3, 4}, reader),
                             [(11, 100, 'pfremote-speaker-3')])
            self.assertEqual(router.decisions(self.graph('Stream/Input/Audio'), uid, {2, 3, 4}, reader),
                             [(11, 100, 'pfremote-mic-3')])
            self.assertIsNone(router.process_display(123, uid, {120: {2, 3}}, root))
            self.assertIsNone(router.process_display(123, uid + 1, {120: {3}}, root))
            (root / '120' / 'environ').write_bytes(b'DISPLAY=:10.0\0')
            self.assertEqual(router.process_display(123, uid, proc_root=root), 10)
            (root / '120' / 'environ').write_bytes(b'DISPLAY=remote:3\0')
            self.assertIsNone(router.process_display(123, uid, proc_root=root))
            (root / '120' / 'environ').write_bytes(b'')
            (root / '120' / 'status').write_text('PPid:\t123\n')
            self.assertIsNone(router.process_display(123, uid, proc_root=root))

    def test_window_discovery_reads_only_pid_properties(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / '120').mkdir()
            uid = (root / '120').stat().st_uid
            def query(args, **kwargs):
                return b'_NET_CLIENT_LIST(WINDOW): window id # 0x12' if args[-1] == '_NET_CLIENT_LIST' else b'_NET_WM_PID(CARDINAL) = 120\n'
            with patch.object(router.subprocess, 'check_output', side_effect=query) as calls:
                self.assertEqual(router.window_processes({2, 3}, uid, root), {120: {2, 3}})
                self.assertTrue(all(c.args[0][-1] in ('_NET_CLIENT_LIST', '_NET_WM_PID') for c in calls.call_args_list))
            with patch.object(router.subprocess, 'check_output', side_effect=OSError):
                self.assertEqual(router.window_processes({3}, uid, root), {})


@unittest.skipUnless(os.name == 'posix', 'Unix socket ownership and permissions')
class RuntimeSocketTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.root.chmod(0o700)
        self.uid = os.getuid()
        (self.root / 'pulse').mkdir(mode=0o700)
        for relative in ['pulse/native', 'pipewire-0']:
            sock = socket.socket(socket.AF_UNIX)
            self.addCleanup(sock.close)
            sock.bind(str(self.root / relative))
        for display in [2, 3, 4]:
            (self.root / f'vnc-{display}').mkdir(mode=0o700)

    def test_existing_apps_reach_both_sockets_and_recreated_runtime_recovers(self):
        created = router.ensure_runtime_audio(self.uid, [2, 3, 4], self.root)
        self.assertEqual(len(created), 6)
        self.assertTrue(all(path.is_socket() and path.is_symlink() for path in created))
        self.assertEqual(router.ensure_runtime_audio(self.uid, [2, 3, 4], self.root), [])
        missing = self.root / 'vnc-3/pulse/native'
        missing.unlink()
        self.assertEqual(router.ensure_runtime_audio(self.uid, [3], self.root), [missing])
        (self.root / 'vnc-3/bus').write_text('unrelated session bus')
        router.remove_runtime_audio(self.uid, [2, 3, 4], self.root)
        self.assertTrue(all(not path.is_symlink() for path in created))
        self.assertEqual((self.root / 'vnc-3/bus').read_text(), 'unrelated session bus')
        self.assertTrue((self.root / 'pulse/native').is_socket())

    def test_never_replaces_existing_endpoint_or_conflicting_link(self):
        native = self.root / 'vnc-3/pulse/native'
        native.parent.mkdir(mode=0o700)
        native.write_text('existing server state')
        pipe = self.root / 'vnc-3/pipewire-0'
        pipe.symlink_to(self.root / 'someone-else')
        self.assertEqual(router.ensure_runtime_audio(self.uid, [3], self.root), [])
        router.remove_runtime_audio(self.uid, [3], self.root)
        self.assertEqual(native.read_text(), 'existing server state')
        self.assertEqual(pipe.readlink(), self.root / 'someone-else')

    def test_rejects_unprotected_or_redirected_directories_and_wrong_owner(self):
        (self.root / 'vnc-2').chmod(0o755)
        (self.root / 'vnc-3/pulse').symlink_to(self.root / 'pulse')
        self.assertEqual(router.ensure_runtime_audio(self.uid + 1, [4], self.root), [])
        created = router.ensure_runtime_audio(self.uid, [2, 3], self.root)
        self.assertEqual(created, [self.root / 'vnc-3/pipewire-0'])
        self.root.chmod(0o755)
        self.assertEqual(router.ensure_runtime_audio(self.uid, [4], self.root), [])


if __name__ == '__main__':
    unittest.main()
