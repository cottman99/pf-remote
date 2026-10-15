import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("shared", Path(__file__).with_name("linux-shared-desktop.py"))
shared = importlib.util.module_from_spec(spec)
spec.loader.exec_module(shared)

BASE = "[Globals]\nsecurity_layer=negotiate\n[Channels]\nrdpsnd=true\ndrdynvc=true\n[Xorg]\nport=-1\n"


class SharedDesktopTests(unittest.TestCase):
    def test_fixed_workspaces_preserve_legacy(self):
        result = shared.render_config(BASE, 1234, [2, 3, 4])
        self.assertTrue(result.startswith(BASE))
        for display in [2, 3, 4]:
            self.assertIn(f"port={5900 + display}\n", result)
            self.assertIn(f"chansrvport=DISPLAY({display},1234)", result)
        self.assertEqual(result.count("password=ask"), 3)
        self.assertNotIn("ip=ask", result)

    def test_reject_invalid_or_conflicting_mapping(self):
        for displays in [[], [2, 2], [0], [100]]:
            with self.assertRaises(ValueError):
                shared.render_config(BASE, 1234, displays)
        for original in [BASE + shared.BEGIN, BASE.replace("negotiate", "rdp"),
                         BASE.replace("rdpsnd=true", "rdpsnd=false"),
                         BASE + "[pfremote-shared-2]\nport=5999\n"]:
            with self.assertRaises(ValueError):
                shared.render_config(original, 1234, [2])

    def test_channel_and_audio_have_independent_lifetimes(self):
        channel = shared.unit(2, 1234, "/home/example")
        audio = shared.unit(2, 1234, "/home/example", True)
        self.assertIn("ExecStart=/usr/sbin/xrdp-chansrv", channel)
        self.assertNotIn("ExecStartPost", channel + audio)
        self.assertIn("BindsTo=pfremote-shared-desktop-2.service", audio)
        self.assertNotIn("Requires=pipewire.service", channel)
        self.assertIn("PartOf=pipewire.service", audio)
        self.assertNotIn("Requires=pipewire.service", shared.router_unit('/home/example', [2, 3]))
        self.assertIn("node.name=pfremote-mic-2", audio)
        self.assertIn('microphone\\" priority.session=100', audio)
        self.assertIn('speakers\\" priority.session=0', audio)
        self.assertIn("node.always-process=true node.pause-on-idle=false node.name=pfremote-speaker-2", audio)
        self.assertNotIn("node.always-process=true node.pause-on-idle=false node.name=pfremote-mic", audio)
        self.assertIn("xrdp_chansrv_audio_out_socket_2", audio)
        self.assertNotIn("set-default", audio)
        with self.assertRaises(ValueError):
            shared.unit(2, 1234, '/home/example%h')

    def test_rollback_preserves_later_unrelated_admin_edits(self):
        added = "\n[OtherDesktop]\nport=5909\n"
        configured = shared.render_config(BASE, 1234, [2, 3, 4]) + added
        restored = shared.remove_managed_config(configured)
        self.assertTrue(restored.startswith(BASE))
        self.assertTrue(restored.endswith(added))
        self.assertNotIn("pfremote-shared-", restored)
        for broken in [BASE, shared.END + shared.BEGIN, configured + shared.BEGIN]:
            with self.assertRaises(ValueError):
                shared.remove_managed_config(broken)


if __name__ == "__main__":
    unittest.main()
