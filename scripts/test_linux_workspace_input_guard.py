import importlib.util
from dataclasses import replace
from pathlib import Path
import sys
import unittest

spec = importlib.util.spec_from_file_location('input_guard', Path(__file__).with_name('linux-workspace-input-guard.py'))
guard = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = guard
spec.loader.exec_module(guard)


class FocusRecoveryTests(unittest.TestCase):
    def sample(self):
        return guard.Snapshot(active=10, focus=20, fallback=20,
            process=(100, '300'), window_manager=(101, '301'), pointer_inside=True,
            normal=True, accepts_input=True, hints_missing=True)

    def test_recovery_requires_click_and_stable_broken_focus(self):
        state = self.sample()
        recovery = guard.ClickRecovery()
        self.assertIsNone(recovery.due(state, 5))
        recovery.click(state, 1234, 10)
        self.assertIsNone(recovery.due(state, 10.05))
        self.assertEqual(recovery.due(state, 10.2), 1234)
        self.assertIsNone(recovery.due(state, 10.3))

    def test_user_selection_native_recovery_and_reused_process_cancel(self):
        original = self.sample()
        changes = [dict(active=11), dict(focus=10), dict(focus=12),
                   dict(advertised=11),
                   dict(process=(100, '999')), dict(window_manager=(101, '999')),
                   dict(pointer_inside=False), dict(normal=False),
                   dict(accepts_input=False), dict(buttons_down=True)]
        for change in changes:
            with self.subTest(change=change):
                recovery = guard.ClickRecovery()
                recovery.click(original, 500, 0)
                self.assertIsNone(recovery.due(replace(original, **change), .2))
                self.assertIsNone(recovery.due(original, .3))

    def test_popups_no_input_unknown_owners_and_stale_click_are_ignored(self):
        original = self.sample()
        for change in [dict(active=0), dict(fallback=0), dict(process=()),
                       dict(window_manager=()), dict(normal=False),
                       dict(accepts_input=False), dict(pointer_inside=False),
                       dict(focus=1), dict(buttons_down=True)]:
            recovery = guard.ClickRecovery()
            state = replace(original, **change)
            recovery.click(state, 500, 0)
            self.assertIsNone(recovery.due(state, .2))
        recovery = guard.ClickRecovery()
        recovery.click(original, 500, 0)
        self.assertIsNone(recovery.due(original, 2))


if __name__ == '__main__':
    unittest.main()
