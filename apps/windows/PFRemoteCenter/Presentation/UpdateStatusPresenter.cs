using PFRemoteCenter.Models;

namespace PFRemoteCenter.Presentation;

internal static class UpdateStatusPresenter
{
    internal static string Create(DoctorResponse doctor, Func<string, string> resource)
    {
        string key = doctor.Checks.FirstOrDefault(check => check.Name == "updates")?.Code switch
        {
            "scheduled" => "UpdatesScheduled",
            "current" => "UpdatesCurrent",
            "downloading" => "UpdatesDownloading",
            "installing" => "UpdatesInstalling",
            "busy" => "UpdatesBusy",
            "incompatible" => "UpdatesIncompatible",
            "held" => "UpdatesHeld",
            "retry" => "UpdatesRetry",
            "available" => "UpdatesAvailable",
            _ => "UpdatesBootstrap",
        };
        return resource(key);
    }
}
