using System.Globalization;

using PFRemoteCenter.Services;

namespace PFRemoteCenter.Presentation;

internal sealed record AgentHandoffPresentation(
    string Status,
    string? NoticeTitle,
    string? NoticeMessage,
    bool CanOpenSettings,
    bool IsSuccess);

internal static class AgentHandoffPresenter
{
    internal static AgentHandoffPresentation Create(
        AgentIntegrationKind integration,
        string computerName,
        Func<string, string> resource)
    {
        if (integration == AgentIntegrationKind.Ready)
        {
            return new(
                string.Format(CultureInfo.CurrentCulture, resource("ContextCopiedStatus"), computerName),
                resource("AgentHandoffReadyTitle"),
                resource("AgentHandoffReadyMessage"),
                false,
                true);
        }

        string messageKey = integration switch
        {
            AgentIntegrationKind.UpdateAvailable => "AgentHandoffUpdateMessage",
            AgentIntegrationKind.Conflict => "AgentHandoffConflictMessage",
            AgentIntegrationKind.Unavailable => "AgentHandoffUnavailableMessage",
            _ => "AgentHandoffSetupMessage",
        };
        return new(
            string.Format(CultureInfo.CurrentCulture, resource("ContextCopiedStatus"), computerName),
            resource("AgentHandoffSetupTitle"),
            resource(messageKey),
            integration != AgentIntegrationKind.Unavailable,
            false);
    }
}
