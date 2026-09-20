using System.Globalization;

using PFRemoteCenter.Models;

namespace PFRemoteCenter.Presentation;

internal static class RecentSessionPresenter
{
	internal static IReadOnlyList<RecentSessionViewModel> Create(
		IReadOnlyList<TargetSummary> targets,
		IReadOnlyList<RecentSessionSummary>? sessions,
		DateTimeOffset now,
		CultureInfo culture,
		Func<string, string> resource)
	{
		if (sessions is null)
		{
			return [];
		}

		Dictionary<string, TargetSummary> targetsByCanonical = targets.ToDictionary(
			target => target.Canonical,
			StringComparer.Ordinal);
		return sessions
			.Take(20)
			.Where(session => targetsByCanonical.ContainsKey(session.CanonicalTarget))
			.GroupBy(session => (session.CanonicalTarget, session.Action))
			.Select(group => CreateGroup(group, targetsByCanonical[group.Key.CanonicalTarget], now, culture, resource))
			.ToArray();
	}

	private static RecentSessionViewModel CreateGroup(
		IGrouping<(string CanonicalTarget, string Action), RecentSessionSummary> group,
		TargetSummary target,
		DateTimeOffset now,
		CultureInfo culture,
		Func<string, string> resource)
	{
		RecentSessionSummary latest = group.First();
		int occurrences = group.Count();
		bool isDesktop = string.Equals(latest.Action, "open", StringComparison.Ordinal);
		string actionLabel = isDesktop
			? string.Format(culture, resource("RecentDesktopAction"), target.Capability.DisplayName)
			: resource("RecentAgentAction");
		string status = string.Equals(latest.Status, "opened", StringComparison.Ordinal)
			? resource("RecentSessionOpenedStatus")
			: resource("RecentSessionCompletedStatus");
		if (occurrences > 1)
		{
			status = string.Format(culture, resource("RecentSessionRepeatedStatus"), status, occurrences);
		}
		bool repeatDesktop = isDesktop &&
			string.Equals(target.Capability.Kind, "desktop", StringComparison.Ordinal) &&
			string.Equals(target.Capability.State, "available", StringComparison.Ordinal);
		bool repeatAgent = string.Equals(latest.Action, "exec", StringComparison.Ordinal) &&
			string.Equals(target.Capability.Kind, "shell", StringComparison.Ordinal) &&
			string.Equals(target.Capability.State, "available", StringComparison.Ordinal);
		return new RecentSessionViewModel(
			latest.SessionId,
			latest.CanonicalTarget,
			latest.Action,
			string.IsNullOrWhiteSpace(target.Device.DisplayName) ? target.Device.Alias : target.Device.DisplayName,
			actionLabel,
			status,
			TimeLabel(latest.StartedAt, now, culture, resource),
			resource(repeatDesktop ? "ReconnectSessionButtonLabel" : "HandAgainToAgentButtonLabel"),
			repeatDesktop || repeatAgent);
	}

	private static string TimeLabel(
		DateTimeOffset startedAt,
		DateTimeOffset now,
		CultureInfo culture,
		Func<string, string> resource)
	{
		TimeSpan elapsed = now.ToUniversalTime() - startedAt.ToUniversalTime();
		if (elapsed < TimeSpan.FromMinutes(1))
		{
			return resource("RecentSessionJustNow");
		}
		if (elapsed < TimeSpan.FromHours(1))
		{
			return string.Format(culture, resource("RecentSessionMinutesAgo"), Math.Max(1, (int)elapsed.TotalMinutes));
		}
		if (elapsed < TimeSpan.FromDays(1))
		{
			return string.Format(culture, resource("RecentSessionHoursAgo"), Math.Max(1, (int)elapsed.TotalHours));
		}
		return startedAt.LocalDateTime.ToString("g", culture);
	}
}
