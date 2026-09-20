using System.Globalization;
using System.Text;
using System.Text.Json;

using PFRemoteCenter.Models;
using PFRemoteCenter.Presentation;
using PFRemoteCenter.Services;

namespace PFRemoteCenter.Tests;

[TestClass]
public sealed class AuthorizationPresenterTests
{
    private static readonly DateTimeOffset Now = new(2026, 8, 18, 12, 0, 0, TimeSpan.Zero);
    private static readonly JsonSerializerOptions WebJson = new(JsonSerializerDefaults.Web);
    private static readonly string[] CliListArguments = ["list", "--json"];
    private static readonly string[] MigrationStatusArguments = ["status-legacy-center"];
    private static readonly string[] MigrationEnableArguments = ["enable-legacy-center"];

    [TestMethod]
    public void DebugContextHandoffExportsWithoutUsingTheUserClipboard()
    {
        string root = Path.Combine(AppContext.BaseDirectory, $"pfremote-context-audit-{Guid.NewGuid():N}");
        string path = Path.Combine(root, "context.txt");
        string? previous = Environment.GetEnvironmentVariable("PFREMOTE_CONTEXT_EXPORT_PATH");
        try
        {
            Environment.SetEnvironmentVariable("PFREMOTE_CONTEXT_EXPORT_PATH", path);
            Assert.IsTrue(ContextHandoffAudit.TryExport("pfremote-context/v1\ntarget: pfremote://fabric-demo/devices/device-compute/capabilities/shell-main\n"));
            StringAssert.Contains(File.ReadAllText(path), "device-compute/capabilities/shell-main");
        }
        finally
        {
            Environment.SetEnvironmentVariable("PFREMOTE_CONTEXT_EXPORT_PATH", previous);
            if (Directory.Exists(root))
            {
                Directory.Delete(root, true);
            }
        }
    }

    [TestMethod]
    public void CatalogModelReadsDesktopProfileFromDaemonJson()
    {
        const string json = """
            {"schema_version":"pfremote.catalog/v1","targets":[{"canonical":"pfremote://fabric-live/devices/device-live-desktop/capabilities/desktop-current","alias":"laptop/desktop","device":{"id":"device-live-desktop","alias":"laptop","display_name":"Laptop","state":"online"},"capability":{"id":"desktop-current","device_id":"device-live-desktop","alias":"desktop","display_name":"Current screen","kind":"desktop","state":"available","features":["clipboard"],"desktop_profile":{"protocol":"vnc","rendering_environment":"physical","authentication":"tailscale-device"}},"granted":true,"authorization":{"status":"active","valid_until":"2026-09-05T09:02:56Z","remaining_seconds":604779}}],"authorization":{"status":"active","valid_until":"2026-09-05T09:02:56Z","remaining_seconds":604779}}
            """;

        CatalogResponse? catalog = JsonSerializer.Deserialize<CatalogResponse>(json, WebJson);

        Assert.IsNotNull(catalog);
        Assert.AreEqual("tailscale-device", catalog.Targets.Single().Capability.DesktopProfile?.Authentication);
    }

    [TestMethod]
    public void ConnectionServicePresenterUsesNonTechnicalReadyLocalAndRetryStates()
    {
        ConnectionServicePresentation ready = ConnectionServicePresenter.Create(
            new DoctorResponse("pfremote.doctor/v1", "development", [new DoctorCheckSummary("connection-service", "pass", "internal")]),
            ConnectionResource);
        ConnectionServicePresentation local = ConnectionServicePresenter.Create(
            new DoctorResponse("pfremote.doctor/v1", "development", [new DoctorCheckSummary("connection-service", "skip", "internal")]),
            ConnectionResource);
        ConnectionServicePresentation retry = ConnectionServicePresenter.Create(
            new DoctorResponse("pfremote.doctor/v1", "development", [new DoctorCheckSummary("connection-service", "pending", "internal")]),
            ConnectionResource);

        Assert.AreEqual(ConnectionServiceNoticeKind.Ready, ready.Kind);
        Assert.AreEqual("ready-title", ready.Title);
        Assert.IsFalse(ready.CanConnect);
        Assert.AreEqual(ConnectionServiceNoticeKind.LocalOnly, local.Kind);
        Assert.AreEqual("local-message", local.Message);
        Assert.IsTrue(local.CanConnect);
        Assert.AreEqual(ConnectionServiceNoticeKind.Retrying, retry.Kind);
        Assert.AreEqual("retry-title", retry.Title);
        Assert.IsTrue(retry.CanConnect);
        Assert.IsFalse(ready.Message.Contains("gateway", StringComparison.OrdinalIgnoreCase));
    }

    [TestMethod]
    public void ConnectionInvitationUsesOneOpaqueFileArgument()
    {
        IReadOnlyList<string> arguments = PfRemoteMigrationClient.CreateConfigureConnectionServiceArguments(@"D:\setup\join.pfremote-link");

        Assert.AreEqual(3, arguments.Count);
        Assert.AreEqual("configure-connection-service", arguments[0]);
        Assert.AreEqual("--invitation", arguments[1]);
        Assert.AreEqual(@"D:\setup\join.pfremote-link", arguments[2]);
    }

    [TestMethod]
    public void ExistingSetupRollbackUsesOneNonTechnicalAction()
    {
        IReadOnlyList<string> arguments = PfRemoteMigrationClient.CreateDisableArguments();

        Assert.AreEqual(1, arguments.Count);
        Assert.AreEqual("disable-legacy-center", arguments[0]);
    }

    [TestMethod]
    public void CliProcessReadsRedirectedJsonAsUtf8()
    {
        var startInfo = PfRemoteCliClient.CreateStartInfo("pfremote.exe", CliListArguments);

        Assert.AreEqual(Encoding.UTF8.WebName, startInfo.StandardOutputEncoding?.WebName);
        Assert.AreEqual(Encoding.UTF8.WebName, startInfo.StandardErrorEncoding?.WebName);
        Assert.IsTrue(startInfo.RedirectStandardOutput);
        Assert.IsTrue(startInfo.RedirectStandardError);
        CollectionAssert.AreEqual(CliListArguments, startInfo.ArgumentList.ToArray());
    }

    [TestMethod]
    public void DaemonLaunchUsesOnlyTheInstalledSiblingWithoutShellOrArguments()
    {
        string path = Path.Combine("C:\\Program Files", "PF Remote Next", "pfremoted.exe");

        System.Diagnostics.ProcessStartInfo startInfo = PfRemoteDaemon.CreateStartInfo(path);

        Assert.AreEqual(path, startInfo.FileName);
        Assert.AreEqual(Path.GetDirectoryName(path), startInfo.WorkingDirectory);
        Assert.IsFalse(startInfo.UseShellExecute);
        Assert.IsTrue(startInfo.CreateNoWindow);
        Assert.AreEqual(0, startInfo.ArgumentList.Count);
    }

	[TestMethod]
	public void InstalledBackgroundRecoveryUsesTheSetupStartupActionWithoutLaunchingAnotherCenter()
	{
		string path = Path.Combine("C:\\Users", "example", "PFRemoteSetup.exe");

		System.Diagnostics.ProcessStartInfo startInfo = PfRemoteLifecycleClient.CreateStartInfo(path, "startup");

		Assert.AreEqual(path, startInfo.FileName);
		Assert.IsFalse(startInfo.UseShellExecute);
		Assert.IsTrue(startInfo.CreateNoWindow);
		Assert.AreEqual(2, startInfo.ArgumentList.Count);
		Assert.AreEqual("startup", startInfo.ArgumentList[0]);
		Assert.AreEqual("--no-launch", startInfo.ArgumentList[1]);
	}

	[TestMethod]
	public void DaemonRecoveryRetriesOnlyWhenTheLocalDaemonWasUnavailable()
	{
		Assert.IsTrue(DaemonRecoveryPolicy.ShouldRecover(new PfRemoteCliException("DAEMON_UNAVAILABLE", "not running")));
		Assert.IsFalse(DaemonRecoveryPolicy.ShouldRecover(new PfRemoteCliException("DESKTOP_OPEN_FAILED", "route failed")));
		Assert.IsFalse(DaemonRecoveryPolicy.ShouldRecover(new InvalidOperationException("other failure")));
	}

	[TestMethod]
	public void BackgroundRecoveryRetriesAfterACooldownWithoutLoopingContinuously()
	{
		DateTimeOffset now = new(2026, 8, 31, 4, 30, 0, TimeSpan.Zero);

		Assert.AreEqual(TimeSpan.FromSeconds(5), BackgroundRecoveryPolicy.HealthProbeInterval);
		Assert.IsTrue(BackgroundRecoveryPolicy.HealthProbeInterval < BackgroundRecoveryPolicy.MinimumRetryInterval);
		Assert.IsTrue(BackgroundRecoveryPolicy.ShouldAttempt(null, now));
		Assert.IsFalse(BackgroundRecoveryPolicy.ShouldAttempt(now, now.AddSeconds(29)));
		Assert.IsTrue(BackgroundRecoveryPolicy.ShouldAttempt(now, now.AddSeconds(30)));
		Assert.IsTrue(BackgroundRecoveryPolicy.ShouldAttempt(now, now.AddMinutes(-1)));
	}

    [TestMethod]
    public void AgentHandoffUsesCanonicalTargetWithoutEmbeddedTask()
    {
        const string canonical = "pfremote://fabric-synthetic/devices/device-synthetic/capabilities/shell-main";

        IReadOnlyList<string> arguments = PfRemoteCliClient.CreateContextArguments(canonical);

        CollectionAssert.AreEqual(new[] { "context", canonical, "--json" }, arguments.ToArray());
    }

	[TestMethod]
	public void AgentHandoffExplainsWhetherCodexIsReadyBeforeTheUserSwitchesApps()
	{
		AgentHandoffPresentation ready = AgentHandoffPresenter.Create(AgentIntegrationKind.Ready, "workstation", AgentHandoffResource);
		AgentHandoffPresentation setup = AgentHandoffPresenter.Create(AgentIntegrationKind.NotEnabled, "workstation", AgentHandoffResource);
		AgentHandoffPresentation unavailable = AgentHandoffPresenter.Create(AgentIntegrationKind.Unavailable, "workstation", AgentHandoffResource);

		Assert.AreEqual("copied workstation", ready.Status);
		Assert.AreEqual("ready-title", ready.NoticeTitle);
		Assert.AreEqual("ready-message", ready.NoticeMessage);
		Assert.IsTrue(ready.IsSuccess);
		Assert.AreEqual("setup-message", setup.NoticeMessage);
		Assert.IsTrue(setup.CanOpenSettings);
		Assert.IsFalse(setup.IsSuccess);
		Assert.AreEqual("unavailable-message", unavailable.NoticeMessage);
		Assert.IsFalse(unavailable.CanOpenSettings);
	}

	[TestMethod]
	public void CodexRegistrationMustPointToTheManagedStableWrapper()
	{
		const string wrapper = @"C:\Users\example\.agents\skills\pf-remote\scripts\pfremote-mcp.ps1";
		string valid = JsonSerializer.Serialize(new
		{
			name = "pf_remote",
			transport = new
			{
				type = "stdio",
				command = "powershell.exe",
				args = AgentIntegrationService.ExpectedMcpArguments(wrapper),
			},
		});
		string conflict = valid.Replace("pfremote-mcp.ps1", "someone-else.ps1", StringComparison.Ordinal);

		Assert.IsTrue(AgentIntegrationService.RegistrationMatches(valid, wrapper));
		Assert.IsFalse(AgentIntegrationService.RegistrationMatches(conflict, wrapper));
		Assert.IsFalse(AgentIntegrationService.RegistrationMatches("{}", wrapper));
	}

	[TestMethod]
	public void CodexSkillRemovalRequiresTheExactPfRemoteOwnershipMarker()
	{
		string root = Path.Combine(AppContext.BaseDirectory, $"pfremote-skill-owner-{Guid.NewGuid():N}");
		Directory.CreateDirectory(root);
		try
		{
			Assert.IsFalse(AgentIntegrationService.IsManagedSkill(root));
			File.WriteAllText(Path.Combine(root, AgentIntegrationService.ManagedMarkerFileName),
				"{\"schema_version\":\"pfremote.codex-integration/v1\",\"product\":\"Another product\"}");
			Assert.IsFalse(AgentIntegrationService.IsManagedSkill(root));
			File.WriteAllText(Path.Combine(root, AgentIntegrationService.ManagedMarkerFileName),
				"{\"schema_version\":\"pfremote.codex-integration/v1\",\"product\":\"PF Remote\"}");
			Assert.IsTrue(AgentIntegrationService.IsManagedSkill(root));
		}
		finally
		{
			Directory.Delete(root, recursive: true);
		}
	}

    [TestMethod]
    public void DesktopChoiceUsesItsExactCanonicalTarget()
    {
        const string canonical = "pfremote://fabric-synthetic/devices/device-synthetic/capabilities/desktop-screen";

        IReadOnlyList<string> arguments = PfRemoteCliClient.CreateOpenArguments(canonical);

        CollectionAssert.AreEqual(new[] { "open", canonical, "--json" }, arguments.ToArray());
    }

    [TestMethod]
    public void DesktopChoiceCanPinOneConnectionToAVisibleRoute()
    {
        const string canonical = "pfremote://fabric-synthetic/devices/device-synthetic/capabilities/desktop-screen";

        IReadOnlyList<string> arguments = PfRemoteCliClient.CreateOpenArguments(canonical, "lan");

        CollectionAssert.AreEqual(new[] { "open", canonical, "--route", "lan", "--json" }, arguments.ToArray());
    }

    [TestMethod]
    public void CliCredentialOpenArgumentsKeepSecretOutOfArguments()
    {
        const string canonical = "pfremote://fabric-synthetic/devices/device-synthetic/capabilities/desktop-screen";

        IReadOnlyList<string> arguments = PfRemoteCliClient.CreateCredentialOpenArguments(canonical);

        CollectionAssert.AreEqual(new[] { "open", canonical, "--credential-stdin", "--json" }, arguments.ToArray());
        Assert.IsFalse(arguments.Any(argument => argument.Contains("password", StringComparison.OrdinalIgnoreCase)));
    }

    [TestMethod]
    public void RecoveryRestoreUsesTheExactSelectedFile()
    {
        const string selected = @"D:\Backups\my recovery.pfremote-recovery";

        IReadOnlyList<string> arguments = PfRemoteRecoveryClient.CreateRestoreArguments(selected);

        CollectionAssert.AreEqual(new[] { "restore", "--file", selected }, arguments.ToArray());
    }

    [TestMethod]
    public void MigrationPreviewReadsOnlyTheExactLegacyCatalog()
    {
        const string selected = @"C:\ProgramData\PFRemoteCenter\catalog.json";

        IReadOnlyList<string> arguments = PfRemoteMigrationClient.CreateExportArguments(selected);

        CollectionAssert.AreEqual(new[] { "export-legacy-center", "--input", selected }, arguments.ToArray());
        CollectionAssert.AreEqual(MigrationStatusArguments, PfRemoteMigrationClient.CreateStatusArguments().ToArray());
        CollectionAssert.AreEqual(MigrationEnableArguments, PfRemoteMigrationClient.CreateEnableArguments().ToArray());
    }

    [TestMethod]
    public void MigrationPreviewHonorsTheDaemonSelectedLegacyCatalog()
    {
        const string selected = @"D:\PFRemoteTest\catalog.json";
        string? previous = Environment.GetEnvironmentVariable("PFREMOTE_LEGACY_CENTER_CATALOG");
        try
        {
            Environment.SetEnvironmentVariable("PFREMOTE_LEGACY_CENTER_CATALOG", selected);
            Assert.AreEqual(selected, PfRemoteMigrationClient.LegacyCenterCatalogPath);
        }
        finally
        {
            Environment.SetEnvironmentVariable("PFREMOTE_LEGACY_CENTER_CATALOG", previous);
        }
    }

    [TestMethod]
    public void MigrationPreviewShowsComputersAndActionsWithoutRoutes()
    {
        var inventory = new LegacyInventoryResponse(
            "pfremote.legacy-inventory/v1",
            [new LegacyDevicePreview(
                "computer-a",
                "Lab computer",
                [new LegacyCapabilityPreview("desktop", "Current screen", "desktop", 3), new LegacyCapabilityPreview("shell", "Automation", "shell", 2)])]);

        MigrationPreviewPresentation result = MigrationPreviewPresenter.Create(
            inventory,
            CultureInfo.InvariantCulture,
            MigrationResource);

        Assert.AreEqual("found 1 2", result.Summary);
        StringAssert.Contains(result.Details, "Lab computer");
        StringAssert.Contains(result.Details, "Current screen · desktop · paths 3");
        StringAssert.Contains(result.Details, "Automation · shell · paths 2");
        Assert.IsFalse(result.Details.Contains("address", StringComparison.OrdinalIgnoreCase));
    }

    [TestMethod]
    public void CreateWithMoreThanOneDayRemainingIsActive()
    {
        AuthorizationPresentation result = AuthorizationPresenter.Create(
            Authorization("active", Now.AddDays(2)),
            Now,
            CultureInfo.InvariantCulture,
            Resource);

        Assert.AreEqual(AuthorizationNoticeKind.Active, result.Kind);
        Assert.AreEqual("active-title", result.Title);
        StringAssert.StartsWith(result.Message, "active-message ");
    }

    [TestMethod]
    public void CreateWithExactlyOneDayRemainingIsExpiring()
    {
        AuthorizationPresentation result = AuthorizationPresenter.Create(
            Authorization("active", Now.Add(AuthorizationPresenter.ExpiringThreshold)),
            Now,
            CultureInfo.InvariantCulture,
            Resource);

        Assert.AreEqual(AuthorizationNoticeKind.Expiring, result.Kind);
        Assert.AreEqual("expiring-title", result.Title);
    }

    [TestMethod]
    public void CreateWithServerExpiredStateIsExpiredEvenBeforeTimestamp()
    {
        AuthorizationPresentation result = AuthorizationPresenter.Create(
            Authorization("expired", Now.AddDays(1)),
            Now,
            CultureInfo.InvariantCulture,
            Resource);

        Assert.AreEqual(AuthorizationNoticeKind.Expired, result.Kind);
    }

    [TestMethod]
    public void CreateWithLocalClockAtBoundaryIsExpired()
    {
        AuthorizationSummary authorization = Authorization("active", Now);

        AuthorizationPresentation result = AuthorizationPresenter.Create(
            authorization,
            Now,
            CultureInfo.InvariantCulture,
            Resource);

        Assert.AreEqual(AuthorizationNoticeKind.Expired, result.Kind);
    }

    [TestMethod]
    public void CreateTargetExpiryProducesVisibleAndAutomationText()
    {
        (string label, string automationName) = AuthorizationPresenter.CreateTargetExpiry(
            Authorization("active", Now.AddDays(1)),
            CultureInfo.InvariantCulture,
            Resource);

        StringAssert.StartsWith(label, "target-label ");
        StringAssert.StartsWith(automationName, "target-automation ");
    }

    [TestMethod]
    public void DeviceCatalogGroupsCapabilitiesAndPrefersShellForAgent()
    {
        AuthorizationSummary authorization = Authorization("active", Now.AddDays(1));
        var device = new DeviceSummary("device-synthetic", "synthetic", "Synthetic Device", "online");
        var shell = new TargetSummary(
            "pfremote://fabric-synthetic/devices/device-synthetic/capabilities/shell-main",
            "synthetic/shell",
            device,
            new CapabilitySummary("shell-main", "shell", "Shell", "shell", "available", null),
            true,
            authorization);
        var desktop = new TargetSummary(
            "pfremote://fabric-synthetic/devices/device-synthetic/capabilities/desktop-main",
            "synthetic/desktop",
            device,
            new CapabilitySummary(
                "desktop-main",
                "desktop",
                "Desktop",
                "desktop",
                "available",
                new DesktopProfileSummary("rdp", "virtual", "windows-sso")),
            true,
            authorization);

        IReadOnlyList<DeviceViewModel> devices = DeviceCatalogPresenter.Create(
            [desktop, shell],
            DeviceResource,
            _ => ("target-label", "target-automation"));

        Assert.HasCount(1, devices);
        Assert.AreEqual("synthetic", devices[0].ToString());
        Assert.AreEqual(shell.Canonical, devices[0].AgentCanonical);
        Assert.HasCount(1, devices[0].DesktopOptions);
        Assert.AreEqual(desktop.Canonical, devices[0].DesktopOptions[0].Canonical);
        Assert.IsTrue(devices[0].DesktopOptions[0].CanOpen);
        Assert.IsFalse(devices[0].DesktopOptions[0].RequiresFirstUseConfirmation);
        Assert.IsTrue(devices[0].CanHandToAgent);
    }

    [TestMethod]
    public void DeviceCatalogHidesTheCurrentComputerFromRemoteDestinations()
    {
        AuthorizationSummary authorization = Authorization("active", Now.AddDays(1));
        var local = new DeviceSummary("device-local", "local", "Local workstation", "online");
        var remote = new DeviceSummary("device-remote", "remote", "Remote workstation", "online");
        TargetSummary Target(DeviceSummary device) => new(
            $"pfremote://fabric-synthetic/devices/{device.Id}/capabilities/desktop-main",
            $"{device.Alias}/desktop",
            device,
            new CapabilitySummary("desktop-main", "desktop", "Desktop", "desktop", "available", new DesktopProfileSummary("rdp", "virtual", "windows-sso")),
            true,
            authorization);

        IReadOnlyList<DeviceViewModel> devices = DeviceCatalogPresenter.Create(
            [Target(local), Target(remote)],
            DeviceResource,
            _ => ("target-label", "target-automation"),
            localComputerName: "local workstation");

        Assert.HasCount(1, devices);
        Assert.AreEqual("remote", devices[0].Alias);
    }

    [TestMethod]
    public void TailscaleBoundRdpExplainsItsFirstUseConfirmation()
    {
        AuthorizationSummary authorization = Authorization("active", Now.AddDays(1));
        var device = new DeviceSummary("device-synthetic", "synthetic", "Synthetic Device", "online");
        var desktop = new TargetSummary(
            "pfremote://fabric-synthetic/devices/device-synthetic/capabilities/desktop-main",
            "synthetic/desktop",
            device,
            new CapabilitySummary("desktop-main", "desktop", "Desktop", "desktop", "available", new DesktopProfileSummary("rdp", "virtual", "tailscale-device")),
            true,
            authorization);

        DesktopOptionViewModel option = DeviceCatalogPresenter.Create(
            [desktop],
            DeviceResource,
            _ => ("target-label", "target-automation")).Single().DesktopOptions.Single();

        Assert.IsTrue(option.CanOpen);
        Assert.IsTrue(option.RequiresFirstUseConfirmation);
    }

    [TestMethod]
    public void DeviceCatalogShowsTheFirstAvailableSmartRoute()
    {
        AuthorizationSummary authorization = Authorization("active", Now.AddDays(1));
        var device = new DeviceSummary("device-synthetic", "synthetic", "Synthetic Device", "online");
        var desktop = new TargetSummary(
            "pfremote://fabric-synthetic/devices/device-synthetic/capabilities/desktop-main",
            "synthetic/desktop",
            device,
            new CapabilitySummary("desktop-main", "desktop", "Desktop", "desktop", "available", new DesktopProfileSummary("rdp", "virtual", "tailscale-device")),
            true,
            authorization,
            RouteOptions:
            [
                new RouteOptionSummary("lan", "unavailable", 1),
                new RouteOptionSummary("tailscale", "available", 2),
                new RouteOptionSummary("frp", "available", 3),
            ]);

        DeviceViewModel result = DeviceCatalogPresenter.Create(
            [desktop],
            DeviceResource,
            _ => ("target-label", "target-automation")).Single();

        Assert.AreEqual("preferred tailscale-route 1", result.ConnectionSummaryLabel);
        Assert.AreEqual("compact-preferred tailscale-route 1", result.CompactConnectionSummaryLabel);
		Assert.AreEqual("Synthetic Device", result.PrimaryDesktop.ComputerName);
    }

    [TestMethod]
    public void DesktopFailureExplainsHowToRecoverFromSmartAndManualRoutes()
    {
        DesktopFailurePresentation smart = DesktopFailurePresenter.Create(
			"DESKTOP_OPEN_FAILED", null, "Friendly Computer", "Desktop 2", FailureResource);
        DesktopFailurePresentation manual = DesktopFailurePresenter.Create(
			"ROUTE_NOT_SELECTABLE", "frp", "Friendly Computer", "Desktop 2", FailureResource);
        DesktopFailurePresentation unavailable = DesktopFailurePresenter.Create(
			"TARGET_NOT_FOUND", null, null, null, FailureResource);

		Assert.AreEqual("named-failure Friendly Computer Desktop 2", smart.Title);
        Assert.AreEqual("smart-recovery", smart.Message);
        Assert.AreEqual("manual-recovery", manual.Message);
		Assert.AreEqual("failure-title", unavailable.Title);
        Assert.AreEqual("refresh-recovery", unavailable.Message);
    }

    [TestMethod]
    public void VncDesktopShowsOneTimeSetupInItsPrimaryAction()
    {
        var authorization = new AuthorizationSummary("active", DateTimeOffset.Now.AddHours(1), 3600);
        var device = new DeviceSummary("device-synthetic", "synthetic", "Synthetic Device", "online");
        var desktop = new TargetSummary(
            "pfremote://fabric-synthetic/devices/device-synthetic/capabilities/desktop-main",
            "synthetic/desktop",
            device,
            new CapabilitySummary("desktop-main", "desktop", "Workspace", "desktop", "available", new DesktopProfileSummary("vnc", "virtual", "tailscale-device")),
            true,
            authorization,
            "setup-required");

        DesktopOptionViewModel option = DeviceCatalogPresenter.Create(
            [desktop],
            DeviceResource,
            _ => ("target-label", "target-automation")).Single().DesktopOptions.Single();

        Assert.AreEqual("setup-desktop", option.Label);
        Assert.IsTrue(option.CanOpen);
    }

    [TestMethod]
    public void DeviceCatalogDisablesActionsThatAreStillBeingConnected()
    {
        AuthorizationSummary authorization = Authorization("active", Now.AddDays(1));
        var device = new DeviceSummary("device-synthetic", "synthetic", "Synthetic Device", "online");
        var shell = new TargetSummary(
            "pfremote://fabric-synthetic/devices/device-synthetic/capabilities/shell-main",
            "synthetic/shell",
            device,
            new CapabilitySummary("shell-main", "shell", "Automation", "shell", "setup-required", null),
            true,
            authorization);
        var desktop = new TargetSummary(
            "pfremote://fabric-synthetic/devices/device-synthetic/capabilities/desktop-main",
            "synthetic/desktop",
            device,
            new CapabilitySummary("desktop-main", "desktop", "Current screen", "desktop", "setup-required", null),
            true,
            authorization);

        DeviceViewModel result = DeviceCatalogPresenter.Create(
            [desktop, shell],
            DeviceResource,
            _ => ("target-label", "target-automation")).Single();

        Assert.IsFalse(result.CanHandToAgent);
        Assert.IsFalse(result.DesktopOptions.Single().CanOpen);
        Assert.IsNull(result.DesktopOptions.Single().Canonical);
        Assert.AreEqual("preparing", result.DesktopOptions.Single().Label);
        Assert.AreEqual("setup-required", result.CapabilityLabel);
    }

    [TestMethod]
    public void DeviceCatalogMarksShellOnlyComputerAsAgentOnly()
    {
        AuthorizationSummary authorization = Authorization("active", Now.AddDays(1));
        var shell = new TargetSummary(
            "pfremote://fabric-synthetic/devices/device-synthetic/capabilities/shell-main",
            "synthetic/shell",
            new DeviceSummary("device-synthetic", "synthetic", "Synthetic Device", "offline"),
            new CapabilitySummary("shell-main", "shell", "Shell", "shell", "available", null),
            true,
            authorization);

        DeviceViewModel device = DeviceCatalogPresenter.Create(
            [shell],
            DeviceResource,
            _ => ("target-label", "target-automation")).Single();

        Assert.HasCount(1, device.DesktopOptions);
        Assert.IsFalse(device.DesktopOptions[0].CanOpen);
        Assert.AreEqual("agent-only", device.CapabilityLabel);
        Assert.AreEqual("offline", device.StatusLabel);
        Assert.AreEqual("no-desktop", device.DesktopOptions[0].Label);
    }

    [TestMethod]
    public void DeviceCatalogCreatesDistinctNamedActionsForMultipleDesktops()
    {
        AuthorizationSummary authorization = Authorization("active", Now.AddDays(1));
        var device = new DeviceSummary("device-synthetic", "synthetic", "Synthetic Device", "online");
        TargetSummary Target(string id, string alias, string name, string environment) => new(
            $"pfremote://fabric-synthetic/devices/device-synthetic/capabilities/{id}",
            $"synthetic/{alias}",
            device,
            new CapabilitySummary(id, alias, name, "desktop", "available", new DesktopProfileSummary("rdp", environment, "windows-sso")),
            true,
            authorization);

        DeviceViewModel result = DeviceCatalogPresenter.Create(
            [Target("desktop-virtual", "workspace", "Engineering workspace", "virtual"), Target("desktop-screen", "screen", "Current screen", "physical")],
            DeviceResource,
            _ => ("target-label", "target-automation")).Single();

        Assert.HasCount(2, result.DesktopOptions);
        var labels = result.DesktopOptions.Select(option => option.Label).ToHashSet(StringComparer.Ordinal);
        Assert.IsTrue(labels.SetEquals(["open RDP · current-screen", "open RDP · independent-desktop"]));
        Assert.AreEqual("desktop-and-agent 2", result.CapabilityLabel);
    }

    [TestMethod]
    public void DeviceCatalogKeepsTargetStableWhenAnotherDesktopWasRecentlyOpened()
    {
        AuthorizationSummary authorization = Authorization("active", Now.AddDays(1));
        var device = new DeviceSummary("device-synthetic", "synthetic", "Synthetic Device", "online");
        TargetSummary Target(string id, string name) => new(
            $"pfremote://fabric-synthetic/devices/device-synthetic/capabilities/{id}",
            $"synthetic/{id}",
            device,
            new CapabilitySummary(id, id, name, "desktop", "available", new DesktopProfileSummary("rdp", "virtual", "windows-sso")),
            true,
            authorization);
        TargetSummary first = Target("desktop-one", "Virtual Desktop :1");
        TargetSummary recent = Target("desktop-two", "Virtual Desktop :2");

        DeviceViewModel result = DeviceCatalogPresenter.Create(
            [first, recent],
            DeviceResource,
            _ => ("target-label", "target-automation"),
            [new RecentSessionSummary("session-recent", recent.Canonical, "open", "opened", Now)]).Single();

        Assert.AreEqual(first.Canonical, result.PrimaryDesktop.Canonical);
        Assert.AreEqual("RDP · independent-desktop 1", result.PrimaryDesktop.DisplayName);
        Assert.AreEqual("choose-desktop", result.PrimaryDesktopActionLabel);
        Assert.AreEqual("", result.PrimaryDesktop.UsageLabel);
        Assert.AreEqual("all-desktops 2", result.AlternateDesktopsLabel);
        Assert.HasCount(2, result.ExpandedDesktops);
        Assert.HasCount(1, result.ExpandedDesktops.Where(desktop => !desktop.IsLastUsed));
        Assert.IsFalse(result.PrimaryDesktop.IsLastUsed);
        Assert.AreNotEqual(result.PrimaryDesktopAutomationId, result.PrimaryDesktop.AutomationId);
        var refreshed = DeviceCatalogPresenter.Create([recent, first], DeviceResource, _ => ("label", "automation")).Single();
        Assert.AreEqual(result.PrimaryDesktopCanonical, refreshed.PrimaryDesktopCanonical);
        Assert.IsNull(DeviceCatalogPresenter.BoundDesktop(result));
        Assert.IsNull(result.PrimaryDesktopCanonical);
        Assert.AreEqual(recent.Canonical, DeviceCatalogPresenter.BoundDesktop(result.DesktopOptions[1])?.Canonical);
        Assert.IsNull(DeviceCatalogPresenter.BoundDesktop(null));
        Assert.IsNull(DeviceCatalogPresenter.BoundDesktop("stale-tag"));
        TargetSummary unavailable = first with { Capability = first.Capability with { State = "unavailable" } };
        var offline = DeviceCatalogPresenter.Create([recent, unavailable], DeviceResource, _ => ("label", "automation")).Single();
        Assert.AreEqual(first.Capability.Id, offline.PrimaryDesktop.AutomationId["desktop-".Length..]);
        Assert.IsTrue(offline.CanOpenPrimaryDesktop, "The chooser must keep reachable siblings available.");
    }

    [TestMethod]
    public void DeviceCatalogUsesFriendlyNamesForNumberedAndXrdpDesktops()
    {
        AuthorizationSummary authorization = Authorization("active", Now.AddDays(1));
        var device = new DeviceSummary("device-synthetic", "synthetic", "Synthetic Device", "online");
        TargetSummary Target(string id, string name) => new(
            $"pfremote://fabric-synthetic/devices/device-synthetic/capabilities/{id}",
            $"synthetic/{id}",
            device,
            new CapabilitySummary(id, id, name, "desktop", "available", new DesktopProfileSummary("rdp", "virtual", "windows-sso")),
            true,
            authorization);

        DeviceViewModel result = DeviceCatalogPresenter.Create(
            [Target("desktop-one", "Virtual Desktop :1"), Target("desktop-linux", "xrdp desktop")],
            DeviceResource,
            _ => ("target-label", "target-automation")).Single();

        var names = result.DesktopOptions.Select(option => option.DisplayName).ToHashSet(StringComparer.Ordinal);
        Assert.IsTrue(names.SetEquals(["RDP · independent-desktop 1", "RDP · remote-workspace"]));
    }

    [TestMethod]
    public void MixedDesktopChooserKeepsAllVncTargetsAndNeverDefaultsToRdp()
    {
        var device = new DeviceSummary("device-example", "example", "Example computer", "online");
        TargetSummary Target(string id, string name, string protocol) => new(
            $"pfremote://fabric-example/devices/device-example/capabilities/{id}",
            $"example/{id}", device,
            new CapabilitySummary(id, id, name, "desktop", "available", new DesktopProfileSummary(protocol, "virtual", "legacy-vnc-password")),
            true, Authorization("active", Now.AddDays(1)));
        TargetSummary rdp = Target("aaa-rdp", "xrdp desktop", "rdp");
        TargetSummary[] vncs = [Target("vnc-two", "Virtual Desktop :2", "vnc"), Target("vnc-three", "Virtual Desktop :3", "vnc"), Target("vnc-four", "Virtual Desktop :4", "vnc")];
        var result = DeviceCatalogPresenter.Create([rdp, .. vncs], DeviceResource, _ => ("label", "automation")).Single();
        Assert.AreEqual("choose-desktop", result.PrimaryDesktopActionLabel);
        Assert.IsNull(result.PrimaryDesktopCanonical);
        Assert.IsNull(DeviceCatalogPresenter.BoundDesktop(result));
        Assert.HasCount(4, result.DesktopOptions);
        CollectionAssert.AreEqual(vncs.Select(v => v.Canonical).ToArray(), result.DesktopOptions.Take(3).Select(v => v.Canonical!).ToArray());
        Assert.IsTrue(result.DesktopOptions.Take(3).All(v => v.DisplayName.StartsWith("TigerVNC · ", StringComparison.Ordinal)));
        foreach (var desktop in result.DesktopOptions)
            Assert.AreEqual(desktop.Canonical, DeviceCatalogPresenter.BoundDesktop(desktop)?.Canonical);
        Assert.AreEqual(rdp.Canonical, result.DesktopOptions[3].Canonical);
    }

	[TestMethod]
	public void DeviceCatalogContentEqualityIgnoresRecreatedObjectsButDetectsRouteChanges()
	{
		AuthorizationSummary authorization = Authorization("active", Now.AddDays(1));
		var device = new DeviceSummary("device-synthetic", "synthetic", "Synthetic Device", "online");
		TargetSummary Target(string routeStatus) => new(
			"pfremote://fabric-synthetic/devices/device-synthetic/capabilities/desktop-main",
			"synthetic/desktop-main",
			device,
			new CapabilitySummary("desktop-main", "desktop-main", "Workspace", "desktop", "available", new DesktopProfileSummary("rdp", "virtual", "windows-sso")),
			true,
			authorization,
			RouteOptions: [new RouteOptionSummary("tailscale", routeStatus, 2)]);

		IReadOnlyList<DeviceViewModel> first = DeviceCatalogPresenter.Create(
			[Target("available")], DeviceResource, _ => ("target-label", "target-automation"));
		IReadOnlyList<DeviceViewModel> recreated = DeviceCatalogPresenter.Create(
			[Target("available")], DeviceResource, _ => ("target-label", "target-automation"));
		IReadOnlyList<DeviceViewModel> changed = DeviceCatalogPresenter.Create(
			[Target("unavailable")], DeviceResource, _ => ("target-label", "target-automation"));

		Assert.IsTrue(DeviceCatalogPresenter.ContentEquals(first, recreated));
		Assert.IsFalse(DeviceCatalogPresenter.ContentEquals(first, changed));
	}

	[TestMethod]
	public void RecentSessionsCollapseRepeatedJourneysAroundTheLatestResult()
	{
		AuthorizationSummary authorization = Authorization("active", Now.AddDays(1));
		var device = new DeviceSummary("device-synthetic", "synthetic", "Synthetic Device", "online");
		var shell = new TargetSummary(
			"pfremote://fabric-synthetic/devices/device-synthetic/capabilities/shell-main",
			"synthetic/shell-main",
			device,
			new CapabilitySummary("shell-main", "shell-main", "Automation", "shell", "available", null),
			true,
			authorization);
		var desktop = new TargetSummary(
			"pfremote://fabric-synthetic/devices/device-synthetic/capabilities/desktop-main",
			"synthetic/desktop-main",
			device,
			new CapabilitySummary("desktop-main", "desktop-main", "Workspace", "desktop", "available", new DesktopProfileSummary("rdp", "virtual", "windows-sso")),
			true,
			authorization);

		IReadOnlyList<RecentSessionViewModel> result = RecentSessionPresenter.Create(
			[shell, desktop],
			[
				new RecentSessionSummary("shell-new", shell.Canonical, "exec", "completed", Now),
				new RecentSessionSummary("desktop", desktop.Canonical, "open", "opened", Now.AddMinutes(-1)),
				new RecentSessionSummary("shell-old", shell.Canonical, "exec", "completed", Now.AddMinutes(-2)),
			],
			Now.AddMinutes(2),
			CultureInfo.InvariantCulture,
			SessionResource);

		Assert.HasCount(2, result);
		Assert.AreEqual("shell-new", result[0].SessionId);
		Assert.AreEqual("Synthetic Device", result[0].ComputerName);
		Assert.AreEqual("completed 2", result[0].StatusLabel);
		Assert.AreEqual("2 minutes ago", result[0].TimeLabel);
		Assert.AreEqual("hand-again", result[0].RepeatActionLabel);
		Assert.AreEqual("opened", result[1].StatusLabel);
		Assert.AreEqual("connect-again", result[1].RepeatActionLabel);
	}

    [TestMethod]
    public void NextRefreshDelaySchedulesTwentyFourHourWarningBoundary()
    {
        TimeSpan? delay = AuthorizationPresenter.NextRefreshDelay(
            Authorization("active", Now.AddDays(3)),
            Now);

        Assert.AreEqual(TimeSpan.FromDays(2), delay);
    }

    [TestMethod]
    public void NextRefreshDelaySchedulesExpiryWhenAlreadyWarning()
    {
        TimeSpan? delay = AuthorizationPresenter.NextRefreshDelay(
            Authorization("active", Now.AddHours(12)),
            Now);

        Assert.AreEqual(TimeSpan.FromHours(12), delay);
    }

    [TestMethod]
    public void NextRefreshDelayStopsAtExpiredOrServerDeniedState()
    {
        Assert.IsNull(AuthorizationPresenter.NextRefreshDelay(Authorization("active", Now), Now));
        Assert.IsNull(AuthorizationPresenter.NextRefreshDelay(Authorization("expired", Now.AddDays(1)), Now));
    }

    [TestMethod]
    public void SingleFlightOperationTrackerRejectsOnlyTheSameActiveTarget()
    {
        var tracker = new SingleFlightOperationTracker();

        Assert.IsTrue(tracker.TryBegin("desktop-one"));
        Assert.IsFalse(tracker.TryBegin("desktop-one"));
        Assert.IsTrue(tracker.TryBegin("desktop-two"));

        tracker.End("desktop-one");

        Assert.IsTrue(tracker.TryBegin("desktop-one"));
    }

    private static AuthorizationSummary Authorization(string status, DateTimeOffset validUntil) =>
        new(status, validUntil, Math.Max(0, (long)(validUntil - Now).TotalSeconds));

    private static string Resource(string key) => key switch
    {
        "AuthorizationActiveTitle" => "active-title",
        "AuthorizationActiveMessage" => "active-message {0}",
        "AuthorizationExpiringTitle" => "expiring-title",
        "AuthorizationExpiringMessage" => "expiring-message {0}",
        "AuthorizationExpiredTitle" => "expired-title",
        "AuthorizationExpiredMessage" => "expired-message {0}",
        "TargetAuthorizationLabel" => "target-label {0}",
        "TargetAuthorizationAutomationName" => "target-automation {0}",
        _ => throw new InvalidOperationException($"Unknown synthetic resource {key}"),
    };

    private static string DeviceResource(string key) => key switch
    {
        "DeviceOnlineLabel" => "online",
        "DeviceOfflineLabel" => "offline",
        "DesktopAndAgentCapabilitiesLabel" => "desktop-and-agent {0}",
        "AgentOnlyCapabilitiesLabel" => "agent-only",
        "SetupRequiredCapabilitiesLabel" => "setup-required",
        "OfflineCapabilitiesLabel" => "offline",
        "OpenDesktopButtonLabel" => "open-desktop",
        "SetupAndOpenDesktopButtonLabel" => "setup-desktop",
        "SetupAndOpenNamedDesktopButtonLabel" => "setup {0}",
        "OpenNamedDesktopButtonLabel" => "open {0}",
        "NoDesktopButtonLabel" => "no-desktop",
        "DesktopSetupRequiredButtonLabel" => "preparing",
        "PhysicalDesktopLabel" => "current-screen",
        "VirtualDesktopLabel" => "independent-desktop",
        "ConnectDesktopButtonLabel" => "connect",
        "OtherDesktopsLabel" => "other-desktops {0}",
        "AllDesktopsLabel" => "all-desktops {0}",
        "ChooseDesktopButtonLabel" => "choose-desktop",
        "LastUsedDesktopLabel" => "last-used",
        "LastConnectedLabel" => "last-connected {0}",
        "NoRecentConnectionLabel" => "no-recent",
        "IndependentDesktopNumberLabel" => "independent-desktop {0}",
        "RemoteWorkspaceLabel" => "remote-workspace",
        "SmartConnectNamedButtonLabel" => "smart-connect {0}",
        "DesktopAvailabilitySummaryLabel" => "desktops {0} {1}",
        "PreferredRouteSummaryLabel" => "preferred {0} {1}",
        "CompactDesktopAvailabilitySummaryLabel" => "compact-desktops {0}",
        "CompactPreferredRouteSummaryLabel" => "compact-preferred {0} {1}",
        "LanRouteLabel" => "lan-route",
        "TailscaleRouteLabel" => "tailscale-route",
        "GatewayRouteLabel" => "gateway-route",
        "ExistingRouteLabel" => "existing-route",
        "SmartConnectRouteLabel" => "smart-route",
        "LastDesktopSummaryLabel" => "last-desktop {0}",
        "ConnectThisDesktopLabel" => "connect-this-desktop",
        _ => throw new InvalidOperationException($"Unknown synthetic resource {key}"),
    };

	private static string SessionResource(string key) => key switch
	{
		"RecentDesktopAction" => "desktop {0}",
		"RecentAgentAction" => "agent",
		"RecentSessionOpenedStatus" => "opened",
		"RecentSessionCompletedStatus" => "completed",
		"RecentSessionRepeatedStatus" => "{0} {1}",
		"RecentSessionJustNow" => "just now",
		"RecentSessionMinutesAgo" => "{0} minutes ago",
		"RecentSessionHoursAgo" => "{0} hours ago",
		"ReconnectSessionButtonLabel" => "connect-again",
		"HandAgainToAgentButtonLabel" => "hand-again",
		_ => throw new InvalidOperationException($"Unknown session resource {key}"),
	};

    private static string FailureResource(string key) => key switch
    {
        "DesktopFailureTitle" => "failure-title",
		"DesktopFailureNamedTitle" => "named-failure {0} {1}",
        "SmartRouteFailedMessage" => "smart-recovery",
        "ManualRouteFailedMessage" => "manual-recovery",
        "DesktopFailureRefreshMessage" => "refresh-recovery",
        _ => throw new InvalidOperationException($"Unknown failure resource {key}"),
    };

    private static string MigrationResource(string key) => key switch
    {
        "MigrationPreviewSummary" => "found {0} {1}",
        "MigrationShellLabel" => "shell",
        "MigrationDesktopLabel" => "desktop",
        "MigrationCapabilityLine" => "{0} · {1} · paths {2}",
        _ => throw new InvalidOperationException($"Unknown synthetic resource {key}"),
    };

    private static string ConnectionResource(string key) => key switch
    {
        "ConnectionServiceReadyTitle" => "ready-title",
        "ConnectionServiceReadyMessage" => "ready-message",
        "ConnectionServiceLocalTitle" => "local-title",
        "ConnectionServiceLocalMessage" => "local-message",
        "ConnectionServiceRetryTitle" => "retry-title",
        "ConnectionServiceRetryMessage" => "retry-message",
        _ => throw new InvalidOperationException($"Unknown connection resource {key}"),
    };

	private static string AgentHandoffResource(string key) => key switch
	{
		"ContextCopiedReadyStatus" => "ready {0}",
		"ContextCopiedStatus" => "copied {0}",
		"AgentHandoffReadyTitle" => "ready-title",
		"AgentHandoffReadyMessage" => "ready-message",
		"AgentHandoffSetupTitle" => "setup-title",
		"AgentHandoffSetupMessage" => "setup-message",
		"AgentHandoffUpdateMessage" => "update-message",
		"AgentHandoffConflictMessage" => "conflict-message",
		"AgentHandoffUnavailableMessage" => "unavailable-message",
		_ => throw new InvalidOperationException($"Unknown Agent handoff resource {key}"),
	};
}
