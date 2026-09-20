using System.Globalization;

using PFRemoteCenter.Models;

namespace PFRemoteCenter.Presentation;

internal enum AuthorizationNoticeKind
{
    Active,
    Expiring,
    Expired,
}

internal sealed record AuthorizationPresentation(
    AuthorizationNoticeKind Kind,
    string Title,
    string Message);

internal static class AuthorizationPresenter
{
    internal static readonly TimeSpan ExpiringThreshold = TimeSpan.FromHours(24);

    internal static TimeSpan? NextRefreshDelay(AuthorizationSummary authorization, DateTimeOffset now)
    {
        if (!string.Equals(authorization.Status, "active", StringComparison.Ordinal))
        {
            return null;
        }

        TimeSpan remaining = authorization.ValidUntil - now;
        if (remaining <= TimeSpan.Zero)
        {
            return null;
        }

        return remaining > ExpiringThreshold
            ? remaining - ExpiringThreshold
            : remaining;
    }

    internal static AuthorizationPresentation Create(
        AuthorizationSummary authorization,
        DateTimeOffset now,
        CultureInfo culture,
        Func<string, string> resource)
    {
        DateTimeOffset validUntil = authorization.ValidUntil.ToLocalTime();
        TimeSpan remaining = authorization.ValidUntil - now;
        string formattedExpiry = validUntil.ToString("g", culture);
        if (!string.Equals(authorization.Status, "active", StringComparison.Ordinal) || remaining <= TimeSpan.Zero)
        {
            return new(
                AuthorizationNoticeKind.Expired,
                resource("AuthorizationExpiredTitle"),
                string.Format(culture, resource("AuthorizationExpiredMessage"), formattedExpiry));
        }

        if (remaining <= ExpiringThreshold)
        {
            return new(
                AuthorizationNoticeKind.Expiring,
                resource("AuthorizationExpiringTitle"),
                string.Format(culture, resource("AuthorizationExpiringMessage"), formattedExpiry));
        }

        return new(
            AuthorizationNoticeKind.Active,
            resource("AuthorizationActiveTitle"),
            string.Format(culture, resource("AuthorizationActiveMessage"), formattedExpiry));
    }

    internal static (string Label, string AutomationName) CreateTargetExpiry(
        AuthorizationSummary authorization,
        CultureInfo culture,
        Func<string, string> resource)
    {
        string formattedExpiry = authorization.ValidUntil.ToLocalTime().ToString("g", culture);
        return (
            string.Format(culture, resource("TargetAuthorizationLabel"), formattedExpiry),
            string.Format(culture, resource("TargetAuthorizationAutomationName"), formattedExpiry));
    }
}
