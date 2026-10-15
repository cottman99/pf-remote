using PFRemoteCenter.Models;
using PFRemoteCenter.Presentation;

namespace PFRemoteCenter.Tests;

[TestClass]
public sealed class DisplayPresetPresenterTests
{
    private static readonly int[] Selection = [2, 0];
    private static DesktopMonitorLayout Layout => new("layout-one", [new(0, 0, 0, 1920, 1080), new(2, 1920, 0, 3840, 1080), new(5, 3840, 0, 5760, 1080)]);

    [TestMethod]
    public void LegacyPresetsKeepTheirWindowPreference()
    {
        Assert.AreEqual("window", DisplayPresetPresenter.Mode(new("", false, false, false)));
        Assert.AreEqual("single", DisplayPresetPresenter.Mode(new("", false, true, false)));
        Assert.AreEqual("all", DisplayPresetPresenter.Mode(new("", false, true, false, Display:new("all"))));
    }

    [TestMethod]
    public void SelectionRequiresCurrentAdjacentScreens()
    {
        Assert.IsNull(DisplayPresetPresenter.SelectionError("selected", [2, 0], Layout));
        Assert.IsNull(DisplayPresetPresenter.SelectionError("selected", [5], Layout));
        Assert.AreEqual("PresetAdjacentScreens", DisplayPresetPresenter.SelectionError("selected", [0, 5], Layout));
        Assert.AreEqual("PresetChooseScreens", DisplayPresetPresenter.SelectionError("selected", [], Layout));
        Assert.AreEqual("PresetDisplaysChanged", DisplayPresetPresenter.SelectionError("selected", [9], Layout));
        Assert.AreEqual("PresetDisplaysUnavailable", DisplayPresetPresenter.SelectionError("selected", [0], null));
        Assert.IsNull(DisplayPresetPresenter.SelectionError("single", [], null));
    }

    [TestMethod]
    public void LayoutChangesDoNotReuseOldMonitorIds()
    {
        var selection = new DesktopDisplaySettings("selected", [2, 0], Layout.Id);
        CollectionAssert.AreEqual(Selection, DisplayPresetPresenter.RestoreSelection(selection, Layout).ToArray());
        Assert.HasCount(0, DisplayPresetPresenter.RestoreSelection(selection, Layout with { Id="changed" }));
        Assert.HasCount(0, DisplayPresetPresenter.RestoreSelection(selection, null));
        var saved = DisplayPresetPresenter.Build("selected", [2, 0], Layout);
        Assert.AreEqual(Layout.Id, saved.LayoutId);
        CollectionAssert.AreEqual(Selection, saved.MonitorIds!.ToArray());
        Assert.IsNull(DisplayPresetPresenter.Build("all", [2, 0], Layout).MonitorIds);
    }
}
