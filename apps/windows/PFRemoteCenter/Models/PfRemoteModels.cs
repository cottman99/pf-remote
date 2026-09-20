using System.Text.Json.Serialization;

namespace PFRemoteCenter.Models;

internal sealed record FleetUpdateResponse(string Status, IReadOnlyList<FleetUpdateReport> Reports);
internal sealed record FleetUpdateReport([property: JsonPropertyName("device_id")] string DeviceId, string Name, string Version, string Status, DateTimeOffset Seen);

internal sealed record DeviceSummary(
    [property: JsonPropertyName("id")] string Id,
    [property: JsonPropertyName("alias")] string Alias,
    [property: JsonPropertyName("display_name")] string DisplayName,
    [property: JsonPropertyName("state")] string State);

internal sealed record CapabilitySummary(
    [property: JsonPropertyName("id")] string Id,
    [property: JsonPropertyName("alias")] string Alias,
    [property: JsonPropertyName("display_name")] string DisplayName,
    [property: JsonPropertyName("kind")] string Kind,
    [property: JsonPropertyName("state")] string State,
    [property: JsonPropertyName("desktop_profile")] DesktopProfileSummary? DesktopProfile);

internal sealed record DesktopProfileSummary(
    [property: JsonPropertyName("protocol")] string Protocol,
    [property: JsonPropertyName("rendering_environment")] string RenderingEnvironment,
    [property: JsonPropertyName("authentication")] string? Authentication,
    [property: JsonPropertyName("visual_effects_policy")] string? VisualEffectsPolicy = null);

internal sealed record AuthorizationSummary(
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("valid_until")] DateTimeOffset ValidUntil,
    [property: JsonPropertyName("remaining_seconds")] long RemainingSeconds);

internal sealed record TargetSummary(
    [property: JsonPropertyName("canonical")] string Canonical,
    [property: JsonPropertyName("alias")] string Alias,
    [property: JsonPropertyName("device")] DeviceSummary Device,
    [property: JsonPropertyName("capability")] CapabilitySummary Capability,
    [property: JsonPropertyName("granted")] bool IsGranted,
    [property: JsonPropertyName("authorization")] AuthorizationSummary Authorization,
    [property: JsonPropertyName("local_setup_state")] string? LocalSetupState = null,
    [property: JsonPropertyName("route_options")] IReadOnlyList<RouteOptionSummary>? RouteOptions = null);

internal sealed record RouteOptionSummary(
    [property: JsonPropertyName("adapter")] string Adapter,
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("order")] int Order);

internal sealed record CatalogResponse(
    [property: JsonPropertyName("schema_version")] string SchemaVersion,
    [property: JsonPropertyName("targets")] IReadOnlyList<TargetSummary> Targets,
    [property: JsonPropertyName("authorization")] AuthorizationSummary Authorization,
    [property: JsonPropertyName("recent_sessions")] IReadOnlyList<RecentSessionSummary>? RecentSessions = null);

internal sealed record RecentSessionSummary(
    [property: JsonPropertyName("session_id")] string SessionId,
    [property: JsonPropertyName("canonical_target")] string CanonicalTarget,
    [property: JsonPropertyName("action")] string Action,
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("started_at")] DateTimeOffset StartedAt);

internal sealed record DoctorCheckSummary(
    [property: JsonPropertyName("name")] string Name,
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("summary")] string Summary,
    [property: JsonPropertyName("code")] string? Code = null);

internal sealed record DoctorResponse(
    [property: JsonPropertyName("schema_version")] string SchemaVersion,
    [property: JsonPropertyName("overall")] string Overall,
    [property: JsonPropertyName("checks")] IReadOnlyList<DoctorCheckSummary> Checks);

internal sealed record DeviceViewModel(
    DeviceSummary Device,
    TargetSummary AgentTarget,
    IReadOnlyList<DesktopOptionViewModel> DesktopOptions,
    string StatusLabel,
	string CapabilityLabel,
	string RecentLabel,
	string ConnectionSummaryLabel,
	string CompactConnectionSummaryLabel,
	string LastDesktopSummaryLabel,
	string AlternateDesktopsLabel,
    string PrimaryDesktopActionLabel,
    string AuthorizationLabel,
    string AuthorizationAutomationName,
	bool IsExpanded = false)
{
    public string Alias => Device.Alias;

    public string DisplayName => Device.DisplayName;

	public string Title => string.IsNullOrWhiteSpace(Device.DisplayName) ? Device.Alias : Device.DisplayName;

	public string Subtitle => string.Equals(Device.DisplayName, Device.Alias, StringComparison.OrdinalIgnoreCase)
		? ""
		: Device.Alias;

	public DesktopOptionViewModel PrimaryDesktop => DesktopOptions[0];

	public string? PrimaryDesktopCanonical => HasAlternateDesktops ? null : PrimaryDesktop.Canonical;

	public string PrimaryDesktopAutomationId => $"primary-{Device.Id}";

	public bool CanOpenPrimaryDesktop => DesktopOptions.Any(option => option.CanOpen);

	public bool HasPrimaryDesktop => DesktopOptions.Any(option => option.Canonical is not null);

	public IReadOnlyList<DesktopOptionViewModel> ExpandedDesktops => DesktopOptions.Where(option => option.Canonical is not null).ToArray();

	public bool IsOnline => string.Equals(Device.State, "online", StringComparison.OrdinalIgnoreCase);

	public bool IsOffline => !IsOnline;

	public bool HasAlternateDesktops => DesktopOptions.Count > 1;

    public string AgentCanonical => AgentTarget.Canonical;

	public string AgentAutomationId => $"agent-{Device.Id}";

	public string DetailsAutomationId => $"details-{Device.Id}";

    public bool CanHandToAgent => string.Equals(AgentTarget.Capability.State, "available", StringComparison.OrdinalIgnoreCase);

    public override string ToString() => Alias;
}

internal sealed record DesktopOptionViewModel(
    string? Canonical,
	string ComputerName,
	string DisplayName,
    string Label,
	string SecondaryActionLabel,
	string UsageLabel,
	string RecentLabel,
	bool IsLastUsed,
    string AutomationId,
    bool CanOpen,
    bool RequiresFirstUseConfirmation,
    IReadOnlyList<RouteOptionSummary> RouteOptions);

internal sealed record RecentSessionViewModel(
    string SessionId,
    string CanonicalTarget,
    string Action,
    string ComputerName,
    string ActionLabel,
    string StatusLabel,
    string TimeLabel,
    string RepeatActionLabel,
    bool CanRepeat)
{
    public string RepeatAutomationId => $"repeat-{SessionId}";
}

internal sealed record InstalledVersionResponse(
    [property: JsonPropertyName("schema_version")] string SchemaVersion,
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("action")] string Action,
    [property: JsonPropertyName("current")] string Current,
    [property: JsonPropertyName("previous")] string? Previous,
    [property: JsonPropertyName("app_path")] string AppPath);

internal sealed record ContextResponse(
    [property: JsonPropertyName("schema_version")] string SchemaVersion,
    [property: JsonPropertyName("envelope")] string Envelope);

internal sealed record ShellActionResponse(
    [property: JsonPropertyName("schema_version")] string SchemaVersion,
    [property: JsonPropertyName("action")] string Action,
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("target")] TargetSummary Target,
    [property: JsonPropertyName("session_id")] string SessionId,
    [property: JsonPropertyName("exit_code")] int ExitCode,
    [property: JsonPropertyName("output")] string? Output,
    [property: JsonPropertyName("error_output")] string? ErrorOutput);

internal sealed record DesktopActionResponse(
    [property: JsonPropertyName("schema_version")] string SchemaVersion,
    [property: JsonPropertyName("action")] string Action,
    [property: JsonPropertyName("status")] string Status,
    [property: JsonPropertyName("target")] TargetSummary Target,
    [property: JsonPropertyName("session_id")] string SessionId,
    [property: JsonPropertyName("protocol")] string Protocol,
    [property: JsonPropertyName("rendering_environment")] string RenderingEnvironment,
    [property: JsonPropertyName("route_adapter")] string? RouteAdapter = null);

internal sealed record LegacyInventoryResponse(
    [property: JsonPropertyName("schema_version")] string SchemaVersion,
    [property: JsonPropertyName("devices")] IReadOnlyList<LegacyDevicePreview> Devices);

internal sealed record LegacyCompatibilityStatus(
    [property: JsonPropertyName("enabled")] bool Enabled);

internal sealed record LegacyDevicePreview(
    [property: JsonPropertyName("name")] string Name,
    [property: JsonPropertyName("display_name")] string DisplayName,
    [property: JsonPropertyName("capabilities")] IReadOnlyList<LegacyCapabilityPreview> Capabilities);

internal sealed record LegacyCapabilityPreview(
    [property: JsonPropertyName("name")] string Name,
    [property: JsonPropertyName("display_name")] string DisplayName,
    [property: JsonPropertyName("kind")] string Kind,
    [property: JsonPropertyName("path_count")] int PathCount);
