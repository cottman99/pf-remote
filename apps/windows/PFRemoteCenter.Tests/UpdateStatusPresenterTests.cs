using PFRemoteCenter.Models;
using PFRemoteCenter.Presentation;

namespace PFRemoteCenter.Tests;

[TestClass]
public sealed class UpdateStatusPresenterTests
{
    [TestMethod]
    public void OlderDaemonDoesNotClaimAutomaticUpdates()
    {
        DoctorResponse doctor = new("pfremote.doctor/v1", "ready", []);
        Assert.AreEqual("UpdatesBootstrap", UpdateStatusPresenter.Create(doctor, key => key));
    }

    [TestMethod]
    public void BusyUpdateUsesLocalizedStateInsteadOfRawServerText()
    {
        DoctorResponse doctor = new("pfremote.doctor/v1", "ready", [new("updates", "pending", "not for display", "busy")]);
        Assert.AreEqual("UpdatesBusy", UpdateStatusPresenter.Create(doctor, key => key));
    }
}
