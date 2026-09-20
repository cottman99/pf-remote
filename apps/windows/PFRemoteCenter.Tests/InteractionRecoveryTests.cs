using System.Collections.ObjectModel;
using System.Collections.Specialized;
using PFRemoteCenter.Models;
using PFRemoteCenter.Presentation;
using PFRemoteCenter.Services;

namespace PFRemoteCenter.Tests;

[TestClass]
public sealed class InteractionRecoveryTests
{
    [TestMethod]
    public async Task ConcurrentDialogsWaitAndFailureReleasesNextDialog()
    {
        var queue = new InteractionQueue();
        var release = new TaskCompletionSource<bool>(TaskCreationOptions.RunContinuationsAsynchronously);
        bool secondShown = false;
        Task<int> first = queue.RunAsync<int>(async () =>
        {
            await release.Task;
            throw new InvalidOperationException("synthetic dialog failure");
        });
        Task<int> second = queue.RunAsync(() => { secondShown = true; return Task.FromResult(2); });
        Assert.IsFalse(secondShown);
        release.SetResult(true);
        try { await first.WaitAsync(TimeSpan.FromSeconds(5)); Assert.Fail("Expected failure"); }
        catch (InvalidOperationException) { }
        Assert.AreEqual(2, await second.WaitAsync(TimeSpan.FromSeconds(5)));
        Assert.IsTrue(secondShown);
        Assert.AreEqual(3, await queue.RunAsync(() => Task.FromResult(3)));
    }

    [TestMethod]
    public void RefreshAndNavigationCannotEraseOperationFeedback()
    {
        var state = new CatalogInteractionState { OperationMessage = "Connection failed" };
        state.CatalogMessage = "Three computers";
        Assert.AreEqual("Connection failed", state.Message);
        state.RecordFailure();
        state.CatalogMessage = "Unavailable";
        Assert.IsTrue(state.IsUnavailable);
        Assert.AreEqual("Connection failed", state.Message);
        state.RecordSuccess();
        Assert.IsFalse(state.IsUnavailable);
        Assert.AreEqual("Connection failed", state.Message);
        state.OperationMessage = "Opened another desktop";
        Assert.AreEqual("Opened another desktop", state.Message);
    }

    [TestMethod]
    public void StaleCatalogPreservesIdentityButCannotOfferOnlineActions()
    {
        var target = new TargetSummary("pfremote://fabric-demo/devices/device-demo/capabilities/desktop-main", "desktop",
            new DeviceSummary("device-demo", "demo", "Demo", "online"),
            new CapabilitySummary("desktop-main", "desktop", "Desktop", "desktop", "available", null),
            true, new AuthorizationSummary("active", DateTimeOffset.UtcNow.AddHours(1), 3600));
        DeviceViewModel original = DeviceCatalogPresenter.Create([target], key => key, _ => ("", ""))[0];
        Assert.IsTrue(original.IsOnline);
        Assert.IsTrue(original.CanOpenPrimaryDesktop);
        DeviceViewModel stale = DeviceCatalogPresenter.AsStale(original, key => key);
        Assert.AreEqual(original.Device.Id, stale.Device.Id);
        Assert.AreEqual(original.PrimaryDesktopCanonical, stale.PrimaryDesktopCanonical);
        Assert.IsFalse(stale.IsOnline);
        Assert.IsFalse(stale.CanOpenPrimaryDesktop);
        Assert.IsFalse(stale.CanHandToAgent);
        Assert.AreEqual("CatalogStaleTitle", stale.StatusLabel);
        Assert.IsTrue(original.CanOpenPrimaryDesktop, "Cached source must remain recoverable");
    }

    private sealed record Row(string Id, string Status);

    [TestMethod]
    public void RefreshUpdatesOnlyChangedRowsAndPreservesUnchangedReferences()
    {
        var first = new Row("a", "online");
        var second = new Row("b", "online");
        var rows = new ObservableCollection<Row> { first, second };
        var changes = new List<NotifyCollectionChangedAction>();
        rows.CollectionChanged += (_, e) => changes.Add(e.Action);
        CollectionReconciler.Update(rows, [new Row("a", "online"), new Row("b", "offline")], row => row.Id, (a, b) => a == b);
        Assert.AreSame(first, rows[0]);
        Assert.AreEqual("offline", rows[1].Status);
        CollectionAssert.AreEqual(new[] { NotifyCollectionChangedAction.Replace }, changes);
        changes.Clear();
        CollectionReconciler.Update(rows, [new Row("b", "offline"), new Row("c", "online")], row => row.Id, (a, b) => a == b);
        Assert.AreEqual("b", rows[0].Id);
        Assert.AreEqual("c", rows[1].Id);
        Assert.IsFalse(changes.Contains(NotifyCollectionChangedAction.Reset));
        CollectionReconciler.Update(rows, Array.Empty<Row>(), row => row.Id, (a, b) => a == b);
        Assert.HasCount(0, rows);
    }

    [TestMethod]
    public void PasswordSaveAndRouteErrorsKeepDifferentRecoveryMessages()
    {
        Assert.AreEqual("DesktopCredentialSaveFailed", DesktopFailurePresenter.Create(
            "DESKTOP_CREDENTIAL_SAVE_FAILED", null, null, null, key => key).Message);
        Assert.AreEqual("ManualRouteFailedMessage", DesktopFailurePresenter.Create(
            "DESKTOP_OPEN_FAILED", "frp", null, null, key => key).Message);
    }
}
