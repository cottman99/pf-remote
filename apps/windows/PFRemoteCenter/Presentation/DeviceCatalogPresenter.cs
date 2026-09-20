using System.Globalization;
using System.Text.RegularExpressions;

using PFRemoteCenter.Models;

namespace PFRemoteCenter.Presentation;

internal static class DeviceCatalogPresenter
{
    internal static DeviceViewModel AsStale(DeviceViewModel device, Func<string, string> resource) => device with
    {
        Device = device.Device with { State = "unknown" },
        AgentTarget = device.AgentTarget with { Capability = device.AgentTarget.Capability with { State = "unavailable" } },
        DesktopOptions = device.DesktopOptions.Select(desktop => desktop with { CanOpen = false }).ToArray(),
        StatusLabel = resource("CatalogStaleTitle"),
        CapabilityLabel = resource("CatalogStaleTitle"),
        ConnectionSummaryLabel = resource("CatalogStaleDescription"),
        CompactConnectionSummaryLabel = resource("CatalogStaleTitle"),
    };

    internal static DesktopOptionViewModel? BoundDesktop(object? model) => model switch
    {
        DeviceViewModel device => device.PrimaryDesktop,
        DesktopOptionViewModel desktop => desktop,
        _ => null,
    };

    internal static bool CanAttemptRoute(IReadOnlyList<RouteOptionSummary> options, string? adapter) =>
        adapter is null || options.Any(option =>
            string.Equals(option.Adapter, adapter, StringComparison.Ordinal) &&
            (string.Equals(option.Status, "available", StringComparison.OrdinalIgnoreCase) ||
             string.Equals(option.Status, "unavailable", StringComparison.OrdinalIgnoreCase)));

	internal static bool ContentEquals(IReadOnlyList<DeviceViewModel> first, IReadOnlyList<DeviceViewModel> second)
	{
		if (first.Count != second.Count)
		{
			return false;
		}
		for (int index = 0; index < first.Count; index++)
		{
			DeviceViewModel left = first[index];
			DeviceViewModel right = second[index];
			if (left.Device != right.Device ||
				left.AgentCanonical != right.AgentCanonical ||
				left.CanHandToAgent != right.CanHandToAgent ||
				left.StatusLabel != right.StatusLabel ||
				left.CapabilityLabel != right.CapabilityLabel ||
				left.RecentLabel != right.RecentLabel ||
				left.ConnectionSummaryLabel != right.ConnectionSummaryLabel ||
				left.CompactConnectionSummaryLabel != right.CompactConnectionSummaryLabel ||
				left.LastDesktopSummaryLabel != right.LastDesktopSummaryLabel ||
				left.AlternateDesktopsLabel != right.AlternateDesktopsLabel ||
				left.PrimaryDesktopActionLabel != right.PrimaryDesktopActionLabel ||
				left.AuthorizationLabel != right.AuthorizationLabel ||
				left.AuthorizationAutomationName != right.AuthorizationAutomationName ||
				!DesktopOptionsEqual(left.DesktopOptions, right.DesktopOptions))
			{
				return false;
			}
		}
		return true;
	}

	private static bool DesktopOptionsEqual(IReadOnlyList<DesktopOptionViewModel> first, IReadOnlyList<DesktopOptionViewModel> second)
	{
		if (first.Count != second.Count)
		{
			return false;
		}
		for (int index = 0; index < first.Count; index++)
		{
			DesktopOptionViewModel left = first[index];
			DesktopOptionViewModel right = second[index];
			if (left.Canonical != right.Canonical ||
				left.ComputerName != right.ComputerName ||
				left.DisplayName != right.DisplayName ||
				left.Label != right.Label ||
				left.SecondaryActionLabel != right.SecondaryActionLabel ||
				left.UsageLabel != right.UsageLabel ||
				left.RecentLabel != right.RecentLabel ||
				left.IsLastUsed != right.IsLastUsed ||
				left.AutomationId != right.AutomationId ||
				left.CanOpen != right.CanOpen ||
				left.RequiresFirstUseConfirmation != right.RequiresFirstUseConfirmation ||
				left.RouteOptions.Count != right.RouteOptions.Count ||
				left.RouteOptions.Where((route, routeIndex) => route != right.RouteOptions[routeIndex]).Any())
			{
				return false;
			}
		}
		return true;
	}

    internal static IReadOnlyList<DeviceViewModel> Create(
        IReadOnlyList<TargetSummary> targets,
        Func<string, string> resource,
        Func<AuthorizationSummary, (string Label, string AutomationName)> authorization,
		IReadOnlyList<RecentSessionSummary>? recentSessions = null,
		string? localComputerName = null)
    {
		Dictionary<string, DateTimeOffset> lastDesktopUse = (recentSessions ?? [])
			.Where(session => string.Equals(session.Action, "open", StringComparison.Ordinal) &&
				string.Equals(session.Status, "opened", StringComparison.Ordinal))
			.GroupBy(session => session.CanonicalTarget, StringComparer.Ordinal)
			.ToDictionary(group => group.Key, group => group.Max(session => session.StartedAt), StringComparer.Ordinal);
        return targets
			.Where(target => string.IsNullOrWhiteSpace(localComputerName) ||
				!string.Equals(target.Device.DisplayName, localComputerName, StringComparison.OrdinalIgnoreCase))
            .GroupBy(target => target.Device.Id, StringComparer.Ordinal)
            .Select(group => CreateDevice(group.ToArray(), resource, authorization, lastDesktopUse))
            .OrderBy(device => device.Alias, StringComparer.CurrentCultureIgnoreCase)
            .ToArray();
    }

    private static DeviceViewModel CreateDevice(
        TargetSummary[] targets,
        Func<string, string> resource,
        Func<AuthorizationSummary, (string Label, string AutomationName)> authorization,
		Dictionary<string, DateTimeOffset> lastDesktopUse)
    {
		TargetSummary first = targets[0];
		TargetSummary[] desktops = targets.Where(target => target.Capability.Kind == "desktop").ToArray();
		TargetSummary[] availableDesktops = desktops.Where(IsAvailable).ToArray();
		TargetSummary agent = targets.FirstOrDefault(target => target.Capability.Kind == "shell" && IsAvailable(target))
			?? desktops.FirstOrDefault(IsAvailable)
			?? targets.FirstOrDefault(IsAvailable)
			?? first;
        AuthorizationSummary earliestAuthorization = targets
            .Select(target => target.Authorization)
            .OrderBy(value => value.ValidUntil)
            .First();
        (string label, string automationName) = authorization(earliestAuthorization);
        string status = string.Equals(first.Device.State, "online", StringComparison.OrdinalIgnoreCase)
            ? resource("DeviceOnlineLabel")
            : resource("DeviceOfflineLabel");
		bool deviceOnline = string.Equals(first.Device.State, "online", StringComparison.OrdinalIgnoreCase);
		string capabilities = !deviceOnline && !targets.Any(IsAvailable)
			? resource("OfflineCapabilitiesLabel")
			: targets.Any(IsAvailable)
			? availableDesktops.Length == 0
				? resource("AgentOnlyCapabilitiesLabel")
				: string.Format(CultureInfo.CurrentCulture, resource("DesktopAndAgentCapabilitiesLabel"), availableDesktops.Length)
			: resource("SetupRequiredCapabilitiesLabel");
		string computerName = string.IsNullOrWhiteSpace(first.Device.DisplayName)
			? first.Device.Alias
			: first.Device.DisplayName;
		DesktopOptionViewModel[] desktopOptions = CreateDesktopOptions(first.Device.Alias, computerName, desktops, resource, lastDesktopUse);
		DateTimeOffset? lastUsed = desktops
			.Where(target => lastDesktopUse.ContainsKey(target.Canonical))
			.Select(target => (DateTimeOffset?)lastDesktopUse[target.Canonical])
			.OrderByDescending(value => value)
			.FirstOrDefault();
		string recent = lastUsed.HasValue
			? string.Format(CultureInfo.CurrentCulture, resource("LastConnectedLabel"), lastUsed.Value.LocalDateTime.ToString("g", CultureInfo.CurrentCulture))
			: resource("NoRecentConnectionLabel");
		string alternateLabel = string.Format(
			CultureInfo.CurrentCulture,
			resource("OtherDesktopsLabel"),
			Math.Max(0, desktopOptions.Length - 1));
		string primaryAction = desktopOptions[0].Canonical is null
			? resource("NoDesktopButtonLabel")
			: string.Format(CultureInfo.CurrentCulture, resource("SmartConnectNamedButtonLabel"), desktopOptions[0].DisplayName);
		RouteOptionSummary? preferredRoute = desktopOptions[0].RouteOptions
			.Where(route => string.Equals(route.Status, "available", StringComparison.OrdinalIgnoreCase))
			.OrderBy(route => route.Order)
			.FirstOrDefault();
		string connectionSummary = availableDesktops.Length == 0
			? capabilities
			: preferredRoute is null
				? string.Format(CultureInfo.CurrentCulture, resource("DesktopAvailabilitySummaryLabel"), availableDesktops.Length, recent)
				: string.Format(
					CultureInfo.CurrentCulture,
					resource("PreferredRouteSummaryLabel"),
					RouteName(preferredRoute.Adapter, resource),
					availableDesktops.Length);
		string compactConnectionSummary = availableDesktops.Length == 0
			? capabilities
			: preferredRoute is null
				? string.Format(CultureInfo.CurrentCulture, resource("CompactDesktopAvailabilitySummaryLabel"), availableDesktops.Length)
				: string.Format(
					CultureInfo.CurrentCulture,
					resource("CompactPreferredRouteSummaryLabel"),
					RouteName(preferredRoute.Adapter, resource),
					availableDesktops.Length);
		string lastDesktopSummary = desktopOptions[0].Canonical is not null && desktopOptions[0].UsageLabel.Length > 0
			? string.Format(CultureInfo.CurrentCulture, resource("LastDesktopSummaryLabel"), desktopOptions[0].DisplayName)
			: recent;

		return new DeviceViewModel(
			first.Device,
			agent,
			desktopOptions,
			status,
			capabilities,
			recent,
			connectionSummary,
			compactConnectionSummary,
			lastDesktopSummary,
			alternateLabel,
			primaryAction,
			label,
			automationName);
	}

	private static DesktopOptionViewModel[] CreateDesktopOptions(
		string deviceAlias,
		string computerName,
		TargetSummary[] desktops,
		Func<string, string> resource,
		Dictionary<string, DateTimeOffset> lastDesktopUse)
	{
		if (desktops.Length == 0)
		{
			string none = resource("NoDesktopButtonLabel");
			return [new DesktopOptionViewModel(null, computerName, none, none, none, "", "", false, $"desktop-none-{deviceAlias}", false, false, [])];
		}

		TargetSummary[] ordered = desktops
			.OrderBy(target => target.Capability.Id, StringComparer.Ordinal)
			.ToArray();
		string? lastUsedCanonical = ordered
			.Where(target => lastDesktopUse.ContainsKey(target.Canonical))
			.OrderByDescending(target => lastDesktopUse[target.Canonical])
			.Select(target => target.Canonical)
			.FirstOrDefault();
		return ordered
		.Select(target =>
			{
				bool ready = IsAvailable(target);
				string name = DesktopName(target, desktops, resource);
				string usage = string.Equals(target.Canonical, lastUsedCanonical, StringComparison.Ordinal)
					? resource("LastUsedDesktopLabel")
					: "";
				string recent = lastDesktopUse.TryGetValue(target.Canonical, out DateTimeOffset lastConnected)
					? string.Format(CultureInfo.CurrentCulture, resource("LastConnectedLabel"), lastConnected.LocalDateTime.ToString("g", CultureInfo.CurrentCulture))
					: resource("NoRecentConnectionLabel");
				return new DesktopOptionViewModel(
				IsAvailable(target) ? target.Canonical : null,
				computerName,
				name,
				ready
					? DesktopButtonLabel(target, desktops.Length == 1 ? null : name, resource)
					: resource("DesktopSetupRequiredButtonLabel"),
				ready ? resource("ConnectThisDesktopLabel") : resource("DesktopSetupRequiredButtonLabel"),
				usage,
				recent,
				string.Equals(target.Canonical, lastUsedCanonical, StringComparison.Ordinal),
				$"desktop-{target.Capability.Id}",
				ready,
				RequiresFirstUseConfirmation(target),
				target.RouteOptions ?? []);
			})
			.ToArray();
	}

	private static bool RequiresFirstUseConfirmation(TargetSummary target) =>
		string.Equals(target.Capability.DesktopProfile?.Protocol, "rdp", StringComparison.OrdinalIgnoreCase) &&
		string.Equals(target.Capability.DesktopProfile?.Authentication, "tailscale-device", StringComparison.OrdinalIgnoreCase);

	private static string DesktopButtonLabel(TargetSummary target, string? name, Func<string, string> resource)
	{
		bool setupRequired = string.Equals(target.LocalSetupState, "setup-required", StringComparison.Ordinal);
		if (name is null)
		{
			return resource(setupRequired ? "SetupAndOpenDesktopButtonLabel" : "OpenDesktopButtonLabel");
		}
		return string.Format(
			CultureInfo.CurrentCulture,
			resource(setupRequired ? "SetupAndOpenNamedDesktopButtonLabel" : "OpenNamedDesktopButtonLabel"),
			name);
	}

	private static bool IsAvailable(TargetSummary target) =>
		string.Equals(target.Capability.State, "available", StringComparison.OrdinalIgnoreCase);

	private static string RouteName(string adapter, Func<string, string> resource) => adapter switch
	{
		"lan" => resource("LanRouteLabel"),
		"tailscale" => resource("TailscaleRouteLabel"),
		"frp" => resource("GatewayRouteLabel"),
		"legacy-external" => resource("ExistingRouteLabel"),
		_ => resource("SmartConnectRouteLabel"),
	};

	private static string DesktopName(TargetSummary target, TargetSummary[] siblings, Func<string, string> resource)
	{
		string visibleName = target.Capability.DisplayName.Trim();
		Match numbered = Regex.Match(visibleName, @"(?i)(?:virtual\s+desktop|虚拟桌面|独立桌面)\s*:?\s*(\d+)$", RegexOptions.CultureInvariant);
		if (numbered.Success)
		{
			return string.Format(CultureInfo.CurrentCulture, resource("IndependentDesktopNumberLabel"), numbered.Groups[1].Value);
		}
		if (visibleName.Contains("xrdp", StringComparison.OrdinalIgnoreCase))
		{
			return resource("RemoteWorkspaceLabel");
		}
		string environment = target.Capability.DesktopProfile?.RenderingEnvironment ?? "";
		bool environmentIsUnique = siblings.Count(sibling =>
			string.Equals(sibling.Capability.DesktopProfile?.RenderingEnvironment, environment, StringComparison.Ordinal)) == 1;
		if (environmentIsUnique && environment == "physical")
		{
			return resource("PhysicalDesktopLabel");
		}
		if (environmentIsUnique && environment == "virtual")
		{
			return resource("VirtualDesktopLabel");
		}
		return visibleName;
	}
}
