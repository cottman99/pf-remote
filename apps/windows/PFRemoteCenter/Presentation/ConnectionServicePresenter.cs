using PFRemoteCenter.Models;

namespace PFRemoteCenter.Presentation;

internal enum ConnectionServiceNoticeKind
{
    Ready,
    LocalOnly,
    Retrying,
}

internal sealed record ConnectionServicePresentation(
    ConnectionServiceNoticeKind Kind,
    string Title,
    string Message,
    bool CanConnect);

internal static class ConnectionServicePresenter
{
    internal static ConnectionServicePresentation Create(
        DoctorResponse doctor,
        Func<string, string> resource)
    {
        DoctorCheckSummary? connection = doctor.Checks.FirstOrDefault(check =>
            string.Equals(check.Name, "connection-service", StringComparison.Ordinal));
        return connection?.Status.ToLowerInvariant() switch
        {
            "pass" => new(ConnectionServiceNoticeKind.Ready, resource("ConnectionServiceReadyTitle"), resource("ConnectionServiceReadyMessage"), false),
            "skip" => new(ConnectionServiceNoticeKind.LocalOnly, resource("ConnectionServiceLocalTitle"), resource("ConnectionServiceLocalMessage"), true),
            _ => new(ConnectionServiceNoticeKind.Retrying, resource("ConnectionServiceRetryTitle"), resource("ConnectionServiceRetryMessage"), true),
        };
    }

    internal static ConnectionServicePresentation Unavailable(Func<string, string> resource) =>
        new(ConnectionServiceNoticeKind.Retrying, resource("ConnectionServiceRetryTitle"), resource("ConnectionServiceRetryMessage"), false);
}
