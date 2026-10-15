using PFRemoteCenter.Models;

namespace PFRemoteCenter.Presentation;

internal static class DisplayPresetPresenter
{
    internal static string Mode(DesktopPresetResponse preset) =>
        preset.Display?.Mode ?? (preset.FullScreen ? "single" : "window");

    internal static IReadOnlyList<int> RestoreSelection(DesktopDisplaySettings? settings, DesktopMonitorLayout? layout) =>
        settings?.Mode == "selected" && layout is not null && settings.LayoutId == layout.Id
            ? settings.MonitorIds?.Where(id => layout.Monitors.Any(m => m.Id == id)).ToArray() ?? []
            : [];

    // This is immediate UI feedback; the daemon rechecks current topology at save and launch.
    internal static string? SelectionError(string mode, IReadOnlyList<int> ids, DesktopMonitorLayout? layout)
    {
        if (mode != "selected") return null;
        if (layout is null || layout.Monitors.Count == 0) return "PresetDisplaysUnavailable";
        if (ids.Count == 0) return "PresetChooseScreens";
        var chosen = layout.Monitors.Where(m => ids.Contains(m.Id)).ToArray();
        if (chosen.Length != ids.Count || ids.Distinct().Count() != ids.Count) return "PresetDisplaysChanged";
        var connected = new HashSet<int> { chosen[0].Id };
        bool changed;
        do
        {
            changed = false;
            foreach (var a in chosen.Where(m => connected.Contains(m.Id)).ToArray())
                foreach (var b in chosen.Where(m => !connected.Contains(m.Id)))
                    if ((Math.Max(a.Left, b.Left) < Math.Min(a.Right, b.Right) && (a.Top == b.Bottom || a.Bottom == b.Top)) ||
                        (Math.Max(a.Top, b.Top) < Math.Min(a.Bottom, b.Bottom) && (a.Left == b.Right || a.Right == b.Left)))
                        changed |= connected.Add(b.Id);
        } while (changed);
        return connected.Count == chosen.Length ? null : "PresetAdjacentScreens";
    }

    internal static DesktopDisplaySettings Build(string mode, IReadOnlyList<int> ids, DesktopMonitorLayout? layout) =>
        mode == "selected" ? new(mode, ids.ToArray(), layout?.Id) : new(mode);
}
