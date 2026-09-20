using System.Globalization;

namespace PFRemoteCenter.Presentation;

internal sealed record DesktopFailurePresentation(string Title, string Message);

internal static class DesktopFailurePresenter
{
    internal static DesktopFailurePresentation Create(
        string? code,
        string? routeAdapter,
		string? computerName,
		string? desktopName,
        Func<string, string> resource)
    {
        bool routeFailure = code is "ROUTE_NOT_SELECTABLE" or "DESKTOP_OPEN_FAILED";
        string messageKey = code is "DESKTOP_CREDENTIAL_SAVE_FAILED" or "DESKTOP_CREDENTIAL_REQUIRED" or "INVALID_CREDENTIAL_INPUT"
            ? "DesktopCredentialSaveFailed"
            : routeFailure
            ? string.IsNullOrWhiteSpace(routeAdapter)
                ? "SmartRouteFailedMessage"
                : "ManualRouteFailedMessage"
            : "DesktopFailureRefreshMessage";
		string title = string.IsNullOrWhiteSpace(computerName) || string.IsNullOrWhiteSpace(desktopName)
			? resource("DesktopFailureTitle")
			: string.Format(CultureInfo.CurrentCulture, resource("DesktopFailureNamedTitle"), computerName, desktopName);
		return new DesktopFailurePresentation(
			title,
            resource(messageKey));
    }
}
