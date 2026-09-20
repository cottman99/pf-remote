using PFRemoteCenter.Services;

namespace PFRemoteCenter.Tests;

[TestClass]
public sealed class LegacyCenterLauncherTests
{
    [TestMethod]
    public void SelectExistingExecutableRejectsCurrentAndUnrelatedApplications()
    {
        string current = Path.GetFullPath(@"C:\Users\Example\PFRemote\versions\alpha.74");
        string legacy = Path.GetFullPath(@"C:\Program Files\PF Remote Center\app\PFRemoteCenter.exe");
        string? selected = LegacyCenterLauncher.SelectExistingExecutable(
            [
                Path.Combine(current, "PFRemoteCenter.exe"),
                @"C:\Program Files\Another Product\Another.exe",
                legacy,
            ],
            current,
            path => string.Equals(path, legacy, StringComparison.OrdinalIgnoreCase) || path.StartsWith(current, StringComparison.OrdinalIgnoreCase));
        Assert.AreEqual(legacy, selected);
    }

    [TestMethod]
    public void ParseDisplayIconAcceptsQuotedExecutableAndResourceIndex()
    {
        Assert.AreEqual(
            @"C:\Program Files\PF Remote Center\app\PFRemoteCenter.exe",
            LegacyCenterLauncher.ParseDisplayIcon(@"""C:\Program Files\PF Remote Center\app\PFRemoteCenter.exe"",0"));
    }
}
