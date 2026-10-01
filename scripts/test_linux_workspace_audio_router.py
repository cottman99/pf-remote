import importlib.util
from pathlib import Path
import unittest

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


if __name__ == '__main__':
    unittest.main()
