using PFRemoteCenter.Models;
using PFRemoteCenter.Presentation;

namespace PFRemoteCenter.Tests;

[TestClass]
public sealed class RouteRetryTests
{
    [TestMethod]
    public void FailedConfiguredRouteRemainsRetryable()
    {
        RouteOptionSummary[] options = [new("frp", "unavailable", 3)];
        Assert.IsTrue(DeviceCatalogPresenter.CanAttemptRoute(options, "frp"));
        Assert.IsFalse(DeviceCatalogPresenter.CanAttemptRoute(options, "lan"));
        Assert.IsTrue(DeviceCatalogPresenter.CanAttemptRoute(options, null));
    }

    [TestMethod]
    public void UnknownOrBlockedRouteIsNotEnabled()
    {
        RouteOptionSummary[] options = [new("frp", "blocked", 3)];
        Assert.IsFalse(DeviceCatalogPresenter.CanAttemptRoute(options, "frp"));
    }
}
