using System.Collections.ObjectModel;
using System.Globalization;

using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Controls.Primitives;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Dispatching;
using Microsoft.Windows.ApplicationModel.Resources;
using PFRemoteCenter.Models;
using PFRemoteCenter.Presentation;
using PFRemoteCenter.Services;
using Windows.ApplicationModel.DataTransfer;
using Windows.Storage;
using Windows.Storage.Pickers;

namespace PFRemoteCenter;

public sealed partial class MainPage : Page
{
    private static readonly TimeSpan MaximumCatalogProbeInterval = TimeSpan.FromMinutes(1);

    private readonly PfRemoteCliClient _client = new();
    private readonly PfRemoteMigrationClient _migrationClient = new();
    private readonly PfRemoteRecoveryClient _recoveryClient = new();
    private readonly PfRemoteLifecycleClient _lifecycleClient = new();
    private readonly AgentIntegrationService _agentIntegrationService = new();
    private readonly LegacyCenterLauncher _legacyCenterLauncher = new();
    private readonly ResourceLoader _resources = new();
    private readonly DispatcherQueueTimer _authorizationTimer;
	private readonly DispatcherQueueTimer _connectionHealthTimer;
	private readonly SingleFlightOperationTracker _desktopOperations = new();
    private bool _isLoaded;
    private bool _refreshInProgress;
	private bool _connectionHealthInProgress;
	private int _visibleOperationCount;
	private DesktopRetryRequest? _desktopRetryRequest;
    private MigrationPreviewPresentation? _migrationPreview;
	private bool _migrationVisibleForSettings;
	private IReadOnlyList<DeviceViewModel> _allDevices = [];
	private string _computerFilter = "all";
	private DateTimeOffset? _lastBackgroundRecoveryAttempt;
	private bool _catalogStateKnown;
    private readonly InteractionQueue _dialogs = new();
    private readonly CatalogInteractionState _catalogFeedback = new();
    private string OperationStatus
    {
        set { _catalogFeedback.OperationMessage = value; StatusTextBlock.Text = _catalogFeedback.Message; }
    }

    private void SetCatalogStatus(string message)
    {
        _catalogFeedback.CatalogMessage = message;
        StatusTextBlock.Text = _catalogFeedback.Message;
    }

    private async Task<ContentDialogResult> ShowDialogAsync(ContentDialog dialog)
    {
        try { return await _dialogs.RunAsync(async () => await dialog.ShowAsync()); }
        catch (Exception)
        {
            string message = _resources.GetString("DialogUnavailable");
            OperationStatus = message;
            if (SettingsHeader.Visibility == Visibility.Visible)
                ShowSettingsStatus(message, InfoBarSeverity.Error);
            else
            {
                AgentHandoffInfoBar.IsOpen = false;
                DesktopActionInfoBar.Title = _resources.GetString("DialogUnavailableTitle");
                DesktopActionInfoBar.Message = message;
                RecoverDesktopButton.Visibility = Visibility.Collapsed;
                DesktopActionInfoBar.IsOpen = true;
            }
            return ContentDialogResult.None;
        }
    }
	private AgentIntegrationKind _agentIntegrationKind = AgentIntegrationKind.Unavailable;
	private readonly HashSet<string> _expandedDeviceIds = new(StringComparer.Ordinal);

    internal ObservableCollection<DeviceViewModel> Devices { get; } = [];
	internal ObservableCollection<RecentSessionViewModel> RecentSessions { get; } = [];
	internal string FilterComputersAutomationName => _resources.GetString("FilterComputersAutomationName");

    public MainPage()
    {
        InitializeComponent();
		InitializeSettingsScrollSurface();
        _authorizationTimer = DispatcherQueue.CreateTimer();
        _authorizationTimer.IsRepeating = false;
        _authorizationTimer.Tick += AuthorizationTimer_Tick;
		_connectionHealthTimer = DispatcherQueue.CreateTimer();
		_connectionHealthTimer.IsRepeating = true;
		_connectionHealthTimer.Interval = BackgroundRecoveryPolicy.HealthProbeInterval;
		_connectionHealthTimer.Tick += ConnectionHealthTimer_Tick;
		RootNavigation.SelectedItem = ComputersNavigationItem;
		ApplyDestination("computers");
		ShowDevelopmentVersion();
    }

	private void InitializeSettingsScrollSurface()
	{
		FrameworkElement[] settingsSections =
		[
			SettingsHeader,
			SettingsActionInfoBar,
			AgentIntegrationPanel,
			ConnectionServicePanel,
			VersionPanel,
			MigrationPanel,
			RecoveryPanel,
		];
		foreach (FrameworkElement section in settingsSections)
		{
			RootContentGrid.Children.Remove(section);
			SettingsContentHost.Children.Add(section);
		}
	}

    private async void OnLoaded(object sender, RoutedEventArgs e)
    {
        _isLoaded = true;
		Task backgroundReady = Task.CompletedTask;
		if (Application.Current is App app)
		{
			backgroundReady = app.DaemonReady;
		}
        await RefreshCatalogAsync();
		try
		{
			// A healthy catalog can become usable while the independent installed
			// background-set check confirms or repairs the Gateway in parallel.
			await backgroundReady;
		}
		catch (Exception)
		{
			// The catalog and connection-service presentations remain the
			// user-facing recovery surfaces when startup repair did not finish.
		}
		_connectionHealthTimer.Start();
        await RefreshMigrationPreviewAsync();
		await RefreshInstalledVersionAsync();
		await RefreshAgentIntegrationAsync();
#if DEBUG
        string? visualAuditState = Environment.GetEnvironmentVariable("PFREMOTE_VISUAL_AUDIT_STATE");
		string? visualAuditTheme = Environment.GetEnvironmentVariable("PFREMOTE_VISUAL_AUDIT_THEME");
		if (string.Equals(visualAuditTheme, "dark", StringComparison.OrdinalIgnoreCase))
		{
			RequestedTheme = ElementTheme.Dark;
		}
		else if (string.Equals(visualAuditTheme, "light", StringComparison.OrdinalIgnoreCase))
		{
			RequestedTheme = ElementTheme.Light;
		}
		string visualAuditComputerName = Devices.FirstOrDefault()?.Title ?? "";
        if (string.Equals(visualAuditState, "repeat-session", StringComparison.OrdinalIgnoreCase))
        {
			RecentSessions.Insert(0, new RecentSessionViewModel(
				"session-visual-repeat",
				"pfremote://fabric-demo/devices/device-studio/capabilities/desktop-main",
				"open",
				"工作室电脑",
				"远程桌面 · 当前屏幕",
				"已打开",
				DateTime.Now.ToString("g", CultureInfo.CurrentCulture),
				_resources.GetString("ReconnectSessionButtonLabel"),
				true));
			RootNavigation.SelectedItem = SessionsNavigationItem;
			ApplyDestination("sessions");
		}
        else if (string.Equals(visualAuditState, "connection-error", StringComparison.OrdinalIgnoreCase))
        {
			string? retryTarget = Devices.SelectMany(device => device.DesktopOptions)
				.Select(desktop => desktop.Canonical)
				.FirstOrDefault(canonical => !string.IsNullOrWhiteSpace(canonical));
			ShowDesktopFailure("DESKTOP_OPEN_FAILED", null, retryTarget);
		}
        else if (string.Equals(visualAuditState, "sessions", StringComparison.OrdinalIgnoreCase))
        {
			RootNavigation.SelectedItem = SessionsNavigationItem;
			ApplyDestination("sessions");
		}
		else if (string.Equals(visualAuditState, "settings", StringComparison.OrdinalIgnoreCase) ||
			string.Equals(visualAuditState, "settings-bottom", StringComparison.OrdinalIgnoreCase) ||
			string.Equals(visualAuditState, "settings-status", StringComparison.OrdinalIgnoreCase))
		{
			RootNavigation.SelectedItem = SettingsNavigationItem;
			ApplyDestination("settings");
		}
		else if (string.Equals(visualAuditState, "empty-computers", StringComparison.OrdinalIgnoreCase))
		{
			_allDevices = [];
			ApplyDeviceFilter();
		}
		else if (string.Equals(visualAuditState, "add-computer", StringComparison.OrdinalIgnoreCase))
		{
			DispatcherQueue.TryEnqueue(async () => await ShowAddComputerDialogAsync());
		}
		else if (string.Equals(visualAuditState, "agent-handoff-warning", StringComparison.OrdinalIgnoreCase))
		{
			AgentHandoffPresentation handoff = AgentHandoffPresenter.Create(
				AgentIntegrationKind.NotEnabled,
				visualAuditComputerName,
				_resources.GetString);
			OperationStatus = handoff.Status;
			AgentHandoffInfoBar.Title = handoff.NoticeTitle;
			AgentHandoffInfoBar.Message = handoff.NoticeMessage;
			AgentHandoffInfoBar.Severity = handoff.IsSuccess ? InfoBarSeverity.Success : InfoBarSeverity.Warning;
			OpenAgentSettingsButton.Visibility = Visibility.Visible;
			AgentHandoffInfoBar.IsOpen = true;
		}
		else if (string.Equals(visualAuditState, "agent-handoff-ready", StringComparison.OrdinalIgnoreCase))
		{
			AgentHandoffPresentation handoff = AgentHandoffPresenter.Create(
				AgentIntegrationKind.Ready,
				visualAuditComputerName,
				_resources.GetString);
			OperationStatus = handoff.Status;
			AgentHandoffInfoBar.Title = handoff.NoticeTitle;
			AgentHandoffInfoBar.Message = handoff.NoticeMessage;
			AgentHandoffInfoBar.Severity = InfoBarSeverity.Success;
			OpenAgentSettingsButton.Visibility = Visibility.Collapsed;
			AgentHandoffInfoBar.IsOpen = true;
		}
		if (string.Equals(visualAuditState, "refreshing", StringComparison.OrdinalIgnoreCase))
		{
			BeginVisibleOperation();
			RefreshComputersButton.IsEnabled = false;
        RetryCatalogButton.IsEnabled = false;
			SetCatalogStatus(_resources.GetString("RefreshingComputersStatus"));
		}
        if (string.Equals(visualAuditState, "recovery", StringComparison.OrdinalIgnoreCase))
        {
            RecoveryPanel.Visibility = Visibility.Visible;
        }
        else if ((string.Equals(visualAuditState, "migration", StringComparison.OrdinalIgnoreCase) ||
                  string.Equals(visualAuditState, "rollback", StringComparison.OrdinalIgnoreCase)) && _migrationPreview is null)
        {
            _migrationPreview = MigrationPreviewPresenter.Create(
                new LegacyInventoryResponse(
                    "pfremote.legacy-inventory/v1",
                    [
                        new LegacyDevicePreview("computer-a", "工作室电脑", [
                            new LegacyCapabilityPreview("desktop", "当前屏幕", "desktop", 3),
                            new LegacyCapabilityPreview("shell", "自动化操作", "shell", 2),
                        ]),
                        new LegacyDevicePreview("computer-b", "随身电脑", [
                            new LegacyCapabilityPreview("desktop", "独立桌面", "desktop", 2),
                        ]),
                    ]),
                CultureInfo.CurrentCulture,
                _resources.GetString);
            bool rollbackAudit = string.Equals(visualAuditState, "rollback", StringComparison.OrdinalIgnoreCase);
            MigrationSummaryText.Text = rollbackAudit ? _resources.GetString("MigrationEnabledSummary") : _migrationPreview.Summary;
            MigrationPreviewButton.IsEnabled = true;
            MigrationEnableButton.Visibility = rollbackAudit ? Visibility.Collapsed : Visibility.Visible;
            MigrationEnableButton.IsEnabled = !rollbackAudit;
            MigrationDisableButton.Visibility = rollbackAudit ? Visibility.Visible : Visibility.Collapsed;
            MigrationDisableButton.IsEnabled = rollbackAudit;
            MigrationPanel.Visibility = Visibility.Visible;
        }
		if (string.Equals(visualAuditState, "settings-bottom", StringComparison.OrdinalIgnoreCase))
		{
			await Task.Delay(100);
			RecoveryPanel.StartBringIntoView();
		}
		else if (string.Equals(visualAuditState, "settings-status", StringComparison.OrdinalIgnoreCase))
		{
			ShowSettingsStatus(_resources.GetString("RecoveryCreatedStatus"), InfoBarSeverity.Success);
		}
		if (!string.Equals(visualAuditState, "add-computer", StringComparison.OrdinalIgnoreCase))
		{
			await VisualAuditCapture.CaptureWhenRequestedAsync(this);
		}
#endif
    }

    private async Task RefreshMigrationPreviewAsync()
    {
        MigrationPanel.Visibility = Visibility.Collapsed;
		_migrationVisibleForSettings = false;
        MigrationPreviewButton.IsEnabled = false;
        MigrationEnableButton.Visibility = Visibility.Collapsed;
        MigrationEnableButton.IsEnabled = false;
        MigrationDisableButton.Visibility = Visibility.Collapsed;
        MigrationDisableButton.IsEnabled = false;
		MigrationOpenLegacyButton.Visibility = _legacyCenterLauncher.IsAvailable ? Visibility.Visible : Visibility.Collapsed;
		MigrationOpenLegacyButton.IsEnabled = _legacyCenterLauncher.IsAvailable;
        try
        {
            LegacyInventoryResponse? inventory = await _migrationClient.PreviewExistingSetupAsync();
            if (inventory is null)
            {
                return;
            }
            _migrationPreview = MigrationPreviewPresenter.Create(inventory, CultureInfo.CurrentCulture, _resources.GetString);
			_migrationVisibleForSettings = true;
            MigrationPreviewButton.IsEnabled = true;
            bool enabled = await _migrationClient.IsExistingSetupEnabledAsync();
            MigrationSummaryText.Text = enabled
                ? _resources.GetString("MigrationEnabledSummary")
                : _migrationPreview.Summary;
            MigrationEnableButton.Visibility = enabled ? Visibility.Collapsed : Visibility.Visible;
            MigrationEnableButton.IsEnabled = !enabled;
            MigrationDisableButton.Visibility = enabled ? Visibility.Visible : Visibility.Collapsed;
            MigrationDisableButton.IsEnabled = enabled;
			MigrationPanel.Visibility = SettingsHeader.Visibility == Visibility.Visible ? Visibility.Visible : Visibility.Collapsed;
        }
        catch (Exception)
        {
            _migrationPreview = null;
			_migrationVisibleForSettings = true;
            MigrationSummaryText.Text = _resources.GetString("MigrationPreviewUnavailable");
			MigrationPanel.Visibility = SettingsHeader.Visibility == Visibility.Visible ? Visibility.Visible : Visibility.Collapsed;
        }
    }

    private async void MigrationEnableButton_Click(object sender, RoutedEventArgs e)
    {
        MigrationEnableButton.IsEnabled = false;
		ShowSettingsStatus(_resources.GetString("MigrationEnablingStatus"), InfoBarSeverity.Informational);
        try
        {
            await _migrationClient.EnableExistingSetupAsync();
            await RefreshCatalogAsync();
            MigrationEnableButton.Visibility = Visibility.Collapsed;
            MigrationDisableButton.Visibility = Visibility.Visible;
            MigrationDisableButton.IsEnabled = true;
            MigrationSummaryText.Text = _resources.GetString("MigrationEnabledSummary");
			ShowSettingsStatus(_resources.GetString("MigrationEnabledStatus"), InfoBarSeverity.Success);
        }
        catch (Exception)
        {
            MigrationEnableButton.IsEnabled = true;
			ShowSettingsStatus(_resources.GetString("MigrationEnableFailedStatus"), InfoBarSeverity.Error);
        }
    }

    private async void MigrationDisableButton_Click(object sender, RoutedEventArgs e)
    {
        MigrationDisableButton.IsEnabled = false;
		ShowSettingsStatus(_resources.GetString("MigrationDisablingStatus"), InfoBarSeverity.Informational);
        try
        {
            await _migrationClient.DisableExistingSetupAsync();
            await RefreshCatalogAsync();
            MigrationDisableButton.Visibility = Visibility.Collapsed;
            MigrationEnableButton.Visibility = Visibility.Visible;
            MigrationEnableButton.IsEnabled = true;
            MigrationSummaryText.Text = _migrationPreview?.Summary ?? _resources.GetString("MigrationDisabledSummary");
			ShowSettingsStatus(_resources.GetString("MigrationDisabledStatus"), InfoBarSeverity.Success);
        }
        catch (Exception)
        {
            MigrationDisableButton.IsEnabled = true;
			ShowSettingsStatus(_resources.GetString("MigrationDisableFailedStatus"), InfoBarSeverity.Error);
        }
    }

    private async void MigrationPreviewButton_Click(object sender, RoutedEventArgs e)
    {
        if (_migrationPreview is null)
        {
            return;
        }
        var details = new TextBlock
        {
            Text = _migrationPreview.Details,
            TextWrapping = TextWrapping.Wrap,
            IsTextSelectionEnabled = true,
        };
        var dialog = new ContentDialog
        {
            XamlRoot = XamlRoot,
            Title = _resources.GetString("MigrationPreviewDialogTitle"),
            Content = new ScrollViewer { Content = details, MaxHeight = 520 },
            CloseButtonText = _resources.GetString("CloseButtonLabel"),
            DefaultButton = ContentDialogButton.Close,
        };
        await ShowDialogAsync(dialog);
    }

	private void MigrationOpenLegacyButton_Click(object sender, RoutedEventArgs e)
	{
		MigrationOpenLegacyButton.IsEnabled = false;
		try
		{
			_legacyCenterLauncher.Open();
			ShowSettingsStatus(_resources.GetString("MigrationOpenLegacyStatus"), InfoBarSeverity.Success);
		}
		catch (Exception)
		{
			ShowSettingsStatus(_resources.GetString("MigrationOpenLegacyFailedStatus"), InfoBarSeverity.Error);
		}
		finally
		{
			MigrationOpenLegacyButton.IsEnabled = _legacyCenterLauncher.IsAvailable;
		}
	}

    private void OnUnloaded(object sender, RoutedEventArgs e)
    {
        _isLoaded = false;
        _authorizationTimer.Stop();
		_connectionHealthTimer.Stop();
    }

    private async void AuthorizationTimer_Tick(DispatcherQueueTimer sender, object args)
    {
        await RefreshCatalogAsync();
    }

	private async void ConnectionHealthTimer_Tick(DispatcherQueueTimer sender, object args)
	{
		if (!_isLoaded || _refreshInProgress || _connectionHealthInProgress)
		{
			return;
		}

		_connectionHealthInProgress = true;
		try
		{
			await RefreshConnectionServiceAsync();
		}
		finally
		{
			_connectionHealthInProgress = false;
		}
	}

    private async Task RefreshCatalogAsync(bool userInitiated = false)
    {
        if (!_isLoaded || _refreshInProgress)
        {
            return;
        }

        _refreshInProgress = true;
		RefreshComputersButton.IsEnabled = false;
        RetryCatalogButton.IsEnabled = false;
		if (userInitiated)
		{
			BeginVisibleOperation();
			SetCatalogStatus(_resources.GetString("RefreshingComputersStatus"));
		}
		try
		{
			CatalogResponse catalog = await RunWithDaemonRecoveryAsync(() => _client.ListAsync());
			_catalogStateKnown = true;
            bool recoveringCatalog = _catalogFeedback.IsUnavailable;
            _catalogFeedback.RecordSuccess();
            CatalogStatusInfoBar.IsOpen = false;
            IReadOnlyList<DeviceViewModel> devices = DeviceCatalogPresenter.Create(
                catalog.Targets,
                _resources.GetString,
				authorization => AuthorizationPresenter.CreateTargetExpiry(
                    authorization,
                    CultureInfo.CurrentCulture,
                    _resources.GetString),
				catalog.RecentSessions,
				Environment.MachineName);
			devices = devices
				.Select(device => device with { IsExpanded = _expandedDeviceIds.Contains(device.Device.Id) })
				.ToArray();
			if (recoveringCatalog || !DeviceCatalogPresenter.ContentEquals(_allDevices, devices))
			{
				_allDevices = devices;
				ApplyDeviceFilter();
			}
            UpdateComputersEmptyState();
			SynchronizeRecentSessions(catalog);

            AuthorizationPresentation authorization = AuthorizationPresenter.Create(
                catalog.Authorization,
                DateTimeOffset.Now,
                CultureInfo.CurrentCulture,
                _resources.GetString);
            AuthorizationInfoBar.Title = authorization.Title;
            AuthorizationInfoBar.Message = authorization.Message;
            AuthorizationInfoBar.Severity = authorization.Kind switch
            {
                AuthorizationNoticeKind.Active => InfoBarSeverity.Success,
                AuthorizationNoticeKind.Expiring => InfoBarSeverity.Warning,
                _ => InfoBarSeverity.Error,
            };
            AutomationProperties.SetName(
                AuthorizationInfoBar,
                string.Format(
                    CultureInfo.CurrentCulture,
                    _resources.GetString("AuthorizationAutomationName"),
                    authorization.Title,
                    authorization.Message));
            AuthorizationInfoBar.IsOpen = authorization.Kind != AuthorizationNoticeKind.Active;

			await RefreshConnectionServiceAsync();

            SetCatalogStatus(authorization.Kind == AuthorizationNoticeKind.Expired
                ? _resources.GetString("CatalogExpiredStatus")
                : string.Format(CultureInfo.CurrentCulture, _resources.GetString("CatalogReady"), Devices.Count));
            ScheduleAuthorizationRefresh(catalog.Authorization);
        }
        catch (Exception)
        {
			_catalogStateKnown = true;
            AuthorizationInfoBar.IsOpen = false;
			ApplyConnectionServicePresentation(ConnectionServicePresenter.Unavailable(_resources.GetString));
			UpdateStatusText.Text = _resources.GetString("UpdatesRetry");
            _catalogFeedback.RecordFailure();
            CatalogStatusInfoBar.Title = _resources.GetString("CatalogStaleTitle");
            CatalogStatusInfoBar.Message = _resources.GetString(_allDevices.Count == 0 ? "CatalogFirstLoadDescription" : "CatalogStaleDescription");
            CatalogStatusInfoBar.IsOpen = true;
            ApplyDeviceFilter();
            foreach (RecentSessionViewModel session in RecentSessions.ToArray())
                RecentSessions[RecentSessions.IndexOf(session)] = session with { CanRepeat = false };
            SetCatalogStatus(_resources.GetString("CatalogUnavailable"));
            ScheduleRefresh(TimeSpan.FromSeconds(30));
        }
        finally
        {
            _refreshInProgress = false;
			RefreshComputersButton.IsEnabled = true;
            RetryCatalogButton.IsEnabled = true;
			if (userInitiated)
			{
				EndVisibleOperation();
			}
        }
    }

	private void BeginVisibleOperation()
	{
		_visibleOperationCount++;
		OperationProgressRing.IsActive = true;
		OperationProgressRing.Visibility = Visibility.Visible;
	}

	private void EndVisibleOperation()
	{
		_visibleOperationCount = Math.Max(0, _visibleOperationCount - 1);
		if (_visibleOperationCount == 0)
		{
			OperationProgressRing.IsActive = false;
			OperationProgressRing.Visibility = Visibility.Collapsed;
		}
	}

	private async Task RefreshConnectionServiceAsync()
	{
		try
		{
			DoctorResponse doctor = await RunWithDaemonRecoveryAsync(() => _client.DoctorAsync());
			UpdateStatusText.Text = UpdateStatusPresenter.Create(doctor, _resources.GetString);
			ConnectionServicePresentation presentation = ConnectionServicePresenter.Create(doctor, _resources.GetString);
			ApplyConnectionServicePresentation(presentation);
			if (presentation.Kind is ConnectionServiceNoticeKind.Ready or ConnectionServiceNoticeKind.LocalOnly)
			{
				_lastBackgroundRecoveryAttempt = null;
			}
			else if (BackgroundRecoveryPolicy.ShouldAttempt(_lastBackgroundRecoveryAttempt, DateTimeOffset.UtcNow) &&
				Application.Current is App app)
			{
				_lastBackgroundRecoveryAttempt = DateTimeOffset.UtcNow;
				try
				{
					await app.EnsureBackgroundStartedAsync();
					ScheduleRefresh(TimeSpan.FromSeconds(5));
				}
				catch (Exception)
				{
					// The visible retry state remains authoritative; a later user
					// refresh can attempt recovery again after the state changes.
				}
			}
		}
		catch (Exception)
		{
			ApplyConnectionServicePresentation(ConnectionServicePresenter.Unavailable(_resources.GetString));
		}
	}

	private static async Task<T> RunWithDaemonRecoveryAsync<T>(Func<Task<T>> operation)
	{
		try
		{
			return await operation();
		}
		catch (Exception exception) when (
			DaemonRecoveryPolicy.ShouldRecover(exception) &&
			Application.Current is App app)
		{
			await app.EnsureBackgroundStartedAsync();
			return await operation();
		}
	}

	private void ComputerSearchBox_TextChanged(AutoSuggestBox sender, AutoSuggestBoxTextChangedEventArgs args)
	{
		if (args.Reason == AutoSuggestionBoxTextChangeReason.UserInput)
		{
			ApplyDeviceFilter();
		}
	}

	private void ComputerFilterItem_Click(object sender, RoutedEventArgs e)
	{
		if (sender is not ToggleMenuFlyoutItem { Tag: string filter })
		{
			return;
		}
		_computerFilter = filter;
		FilterAllComputersItem.IsChecked = filter == "all";
		FilterOnlineComputersItem.IsChecked = filter == "online";
		FilterOfflineComputersItem.IsChecked = filter == "offline";
		ApplyDeviceFilter();
	}

	private void ApplyDeviceFilter()
	{
		string query = ComputerSearchBox?.Text.Trim() ?? "";
		IEnumerable<DeviceViewModel> visible = _allDevices.Where(device =>
			(_computerFilter == "all" || (_computerFilter == "online" && device.IsOnline) || (_computerFilter == "offline" && device.IsOffline)) &&
			(query.Length == 0 || device.Title.Contains(query, StringComparison.CurrentCultureIgnoreCase) ||
			 device.Alias.Contains(query, StringComparison.CurrentCultureIgnoreCase)));
        DeviceViewModel[] next = visible.Select(device => device with { IsExpanded = _expandedDeviceIds.Contains(device.Device.Id) })
            .Select(device => _catalogFeedback.IsUnavailable
            ? DeviceCatalogPresenter.AsStale(device, _resources.GetString) : device).ToArray();
        CollectionReconciler.Update(Devices, next, device => device.Device.Id,
            (left, right) => left.IsExpanded == right.IsExpanded && DeviceCatalogPresenter.ContentEquals([left], [right]));
		UpdateComputersEmptyState();
		if (_isLoaded && ComputersHeader.Visibility == Visibility.Visible)
		{
			SetCatalogStatus(_catalogFeedback.IsUnavailable ? _resources.GetString("CatalogUnavailable")
                : string.Format(CultureInfo.CurrentCulture, _resources.GetString("CatalogReady"), Devices.Count));
		}
	}

	private void UpdateComputersEmptyState()
	{
		bool noVisibleDevices = Devices.Count == 0;
		bool noKnownDevices = _allDevices.Count == 0;
		ComputersEmptyHeading.Text = _resources.GetString(_catalogFeedback.IsUnavailable ? "CatalogStaleTitle" : noKnownDevices
			? "ComputersEmptyHeading"
			: "ComputersNoMatchHeading");
		ComputersEmptyDescription.Text = _resources.GetString(_catalogFeedback.IsUnavailable ? (noKnownDevices ? "CatalogFirstLoadDescription" : "CatalogStaleDescription") : noKnownDevices
			? "ComputersEmptyDescription"
			: "ComputersNoMatchDescription");
		ComputersEmptyActionButton.Content = _resources.GetString(_catalogFeedback.IsUnavailable ? "RetryCatalogButton/Content" : noKnownDevices
			? "ComputersEmptySyncButtonLabel"
			: "ComputersNoMatchResetButtonLabel");
		AutomationProperties.SetName(ComputersEmptyActionButton, ComputersEmptyActionButton.Content?.ToString() ?? string.Empty);
		bool computersVisible = ComputersHeader.Visibility == Visibility.Visible;
		ComputersEmptyState.Visibility = computersVisible && _catalogStateKnown && noVisibleDevices ? Visibility.Visible : Visibility.Collapsed;
		DevicesList.Visibility = computersVisible && !noVisibleDevices ? Visibility.Visible : Visibility.Collapsed;
	}

	private async void ComputersEmptyActionButton_Click(object sender, RoutedEventArgs e)
	{
		if (_catalogFeedback.IsUnavailable)
        {
            await RefreshCatalogAsync(userInitiated: true);
            return;
        }
        if (_allDevices.Count == 0)
		{
			await ShowAddComputerDialogAsync();
			return;
		}

		ComputerSearchBox.Text = string.Empty;
		_computerFilter = "all";
		FilterAllComputersItem.IsChecked = true;
		FilterOnlineComputersItem.IsChecked = false;
		FilterOfflineComputersItem.IsChecked = false;
		ApplyDeviceFilter();
	}

	private async void AddComputerButton_Click(object sender, RoutedEventArgs e)
	{
		await ShowAddComputerDialogAsync();
	}

	private async Task ShowAddComputerDialogAsync()
	{
		var computerName = new TextBox
		{
			Header = _resources.GetString("NewComputerNameLabel"),
			PlaceholderText = _resources.GetString("NewComputerNamePlaceholder"),
			MaxLength = 120,
		};
		var operatingSystem = CreateNewComputerChoice("NewComputerOperatingSystemLabel",
			"NewComputerUnknownOption", "NewComputerWindowsOption", "NewComputerLinuxOption", "NewComputerMacOption");
		var intendedUse = CreateNewComputerChoice("NewComputerUseLabel",
			"NewComputerUseUnknownOption", "NewComputerUseBothOption", "NewComputerUseDesktopOption", "NewComputerUseAgentOption");
		var notes = new TextBox
		{
			Header = _resources.GetString("NewComputerNotesLabel"),
			PlaceholderText = _resources.GetString("NewComputerNotesPlaceholder"),
			AcceptsReturn = true,
			TextWrapping = TextWrapping.Wrap,
			MaxLength = 500,
			MinHeight = 72,
		};
		var privacy = new InfoBar
		{
			IsOpen = true,
			IsClosable = false,
			Severity = InfoBarSeverity.Informational,
			Title = _resources.GetString("NewComputerPrivacyTitle"),
			Message = _resources.GetString("NewComputerPrivacyMessage"),
		};
		var error = new TextBlock
		{
			Text = _resources.GetString("NewComputerSecretDetectedMessage"),
			Foreground = (Microsoft.UI.Xaml.Media.Brush)Application.Current.Resources["SystemFillColorCriticalBrush"],
			TextWrapping = TextWrapping.Wrap,
			Visibility = Visibility.Collapsed,
		};
		var form = new StackPanel { Spacing = 12, MinWidth = 240 };
		form.Children.Add(new TextBlock
		{
			Text = _resources.GetString("NewComputerDialogDescription"),
			TextWrapping = TextWrapping.Wrap,
			Foreground = (Microsoft.UI.Xaml.Media.Brush)Application.Current.Resources["TextFillColorSecondaryBrush"],
		});
		form.Children.Add(privacy);
		form.Children.Add(error);
		form.Children.Add(computerName);
		form.Children.Add(operatingSystem);
		form.Children.Add(intendedUse);
		form.Children.Add(notes);

		bool copied = false;
		var dialog = new ContentDialog
		{
			XamlRoot = XamlRoot,
			Title = _resources.GetString("NewComputerDialogTitle"),
			Content = new ScrollViewer
			{
				Content = form,
				MaxHeight = 580,
				HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled,
				VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
			},
			PrimaryButtonText = _resources.GetString("NewComputerCopyAgentTaskButtonLabel"),
			CloseButtonText = _resources.GetString("CancelButtonLabel"),
			DefaultButton = ContentDialogButton.Primary,
		};
#if DEBUG
		if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_VISUAL_AUDIT_STATE"), "add-computer", StringComparison.OrdinalIgnoreCase))
		{
			dialog.Opened += async (_, _) => await VisualAuditCapture.CaptureWhenRequestedAsync(dialog);
		}
#endif
		dialog.PrimaryButtonClick += (_, args) =>
		{
			var input = new NewComputerHandoffInput(
				computerName.Text,
				SelectedChoice(operatingSystem),
				SelectedChoice(intendedUse),
				_resources.GetString("NewComputerRouteAutomaticOption"),
				_resources.GetString("NewComputerInstallUnknownOption"),
				string.Empty,
				notes.Text);
			if (NewComputerHandoffPresenter.ContainsLikelySecret(input))
			{
				args.Cancel = true;
				error.Visibility = Visibility.Visible;
				return;
			}

			try
			{
				var package = new DataPackage();
				package.SetText(NewComputerHandoffPresenter.Create(input, _resources.GetString));
				Clipboard.SetContent(package);
				Clipboard.Flush();
				copied = true;
			}
			catch (Exception)
			{
				args.Cancel = true;
				error.Text = _resources.GetString("NewComputerCopyFailedMessage");
				error.Visibility = Visibility.Visible;
			}
		};
		await ShowDialogAsync(dialog);
		if (copied)
		{
			OperationStatus = _resources.GetString("NewComputerCopiedStatus");
			AgentHandoffInfoBar.Title = _resources.GetString("NewComputerCopiedTitle");
			AgentHandoffInfoBar.Message = _resources.GetString("NewComputerCopiedMessage");
			AgentHandoffInfoBar.Severity = InfoBarSeverity.Success;
			OpenAgentSettingsButton.Visibility = Visibility.Collapsed;
			AgentHandoffInfoBar.IsOpen = true;
		}
	}

	private ComboBox CreateNewComputerChoice(string headerKey, params string[] optionKeys)
	{
		var combo = new ComboBox
		{
			Header = _resources.GetString(headerKey),
			HorizontalAlignment = HorizontalAlignment.Stretch,
		};
		foreach (string key in optionKeys)
		{
			combo.Items.Add(_resources.GetString(key));
		}
		combo.SelectedIndex = 0;
		return combo;
	}

	private static string SelectedChoice(ComboBox combo) => combo.SelectedItem?.ToString() ?? string.Empty;

	private void ApplyConnectionServicePresentation(ConnectionServicePresentation presentation)
	{
		ConnectionServiceInfoBar.Title = presentation.Title;
		ConnectionServiceInfoBar.Message = presentation.Message;
		ConnectionServiceInfoBar.Severity = presentation.Kind switch
		{
			ConnectionServiceNoticeKind.Ready => InfoBarSeverity.Success,
			ConnectionServiceNoticeKind.LocalOnly => InfoBarSeverity.Informational,
			_ => InfoBarSeverity.Warning,
		};
		ConnectionServiceButton.Visibility = presentation.CanConnect ? Visibility.Visible : Visibility.Collapsed;
		if (presentation.CanConnect)
		{
			bool replacing = presentation.Kind == ConnectionServiceNoticeKind.Retrying;
			ConnectionServiceButton.Content = _resources.GetString(replacing
				? "ConnectionServiceReplaceButtonLabel"
				: "ConnectionServiceConnectButtonLabel");
			AutomationProperties.SetName(ConnectionServiceButton, _resources.GetString(replacing
				? "ConnectionServiceReplaceButtonAutomationName"
				: "ConnectionServiceConnectButtonAutomationName"));
		}
		AutomationProperties.SetName(
			ConnectionServiceInfoBar,
			string.Format(
				CultureInfo.CurrentCulture,
				_resources.GetString("ConnectionServiceAutomationName"),
				presentation.Title,
				presentation.Message));
		ConnectionServiceInfoBar.IsOpen = true;
	}

	private void ShowSettingsStatus(string message, InfoBarSeverity severity)
	{
		SettingsActionInfoBar.Message = message;
		SettingsActionInfoBar.Severity = severity;
		AutomationProperties.SetName(SettingsActionInfoBar, message);
		SettingsActionInfoBar.IsOpen = true;
	}

	private async void ConnectionServiceButton_Click(object sender, RoutedEventArgs e)
	{
		var picker = new FileOpenPicker { SuggestedStartLocation = PickerLocationId.DocumentsLibrary };
		picker.FileTypeFilter.Add(".pfremote-link");
		InitializePicker(picker);
		StorageFile? file = await picker.PickSingleFileAsync();
		if (file is null)
		{
			return;
		}
		ConnectionServiceButton.IsEnabled = false;
		ShowSettingsStatus(_resources.GetString("ConnectionServiceConnectingStatus"), InfoBarSeverity.Informational);
		try
		{
			await _migrationClient.ConfigureConnectionServiceAsync(file.Path);
			ShowSettingsStatus(_resources.GetString("ConnectionServiceConnectedStatus"), InfoBarSeverity.Success);
			ApplyConnectionServicePresentation(new ConnectionServicePresentation(
				ConnectionServiceNoticeKind.Retrying,
				_resources.GetString("ConnectionServiceRetryTitle"),
				_resources.GetString("ConnectionServiceRetryMessage"),
				true));
			ScheduleRefresh(TimeSpan.FromSeconds(5));
		}
		catch (Exception)
		{
			ShowSettingsStatus(_resources.GetString("ConnectionServiceFailedStatus"), InfoBarSeverity.Error);
			ConnectionServiceButton.IsEnabled = true;
		}
	}

    private void ScheduleAuthorizationRefresh(AuthorizationSummary authorization)
    {
        TimeSpan delay = AuthorizationPresenter.NextRefreshDelay(authorization, DateTimeOffset.Now)
            ?? TimeSpan.FromMinutes(1);
        ScheduleRefresh(delay);
    }

    private void ScheduleRefresh(TimeSpan delay)
    {
        if (!_isLoaded)
        {
            return;
        }

        _authorizationTimer.Stop();
		TimeSpan boundedDelay = delay > MaximumCatalogProbeInterval
			? MaximumCatalogProbeInterval
            : delay;
        _authorizationTimer.Interval = boundedDelay < TimeSpan.FromSeconds(1)
            ? TimeSpan.FromSeconds(1)
            : boundedDelay;
        _authorizationTimer.Start();
    }

    private static DesktopOptionViewModel? BoundDesktop(SplitButton button) =>
        DeviceCatalogPresenter.BoundDesktop(button.DataContext);

    private void RouteSplitButton_DataContextChanged(FrameworkElement sender, DataContextChangedEventArgs args)
    {
        if (sender is not SplitButton button) return;
        button.Flyout?.Hide();
        if (BoundDesktop(button) is { } desktop)
            AutomationProperties.SetName(button, string.Format(CultureInfo.CurrentCulture,
                _resources.GetString("SmartConnectButtonAutomationName"), desktop.ComputerName, desktop.DisplayName));
    }

    private void RouteSplitButton_Loaded(object sender, RoutedEventArgs e)
    {
        if (sender is not SplitButton splitButton) return;
        var flyout = new MenuFlyout();
        flyout.Opening += (_, _) =>
        {
            // Recycled controls must resolve their current model, never a Loaded-time target.
            flyout.Items.Clear();
            if (BoundDesktop(splitButton)?.Canonical is string canonical &&
                FindDesktop(canonical) is { CanOpen: true } desktop)
                PopulateRouteMenu(flyout, desktop);
        };
        splitButton.Flyout = flyout;
        if (BoundDesktop(splitButton) is { } bound)
            AutomationProperties.SetName(splitButton, string.Format(
                CultureInfo.CurrentCulture,
                _resources.GetString("SmartConnectButtonAutomationName"),
                bound.ComputerName, bound.DisplayName));
    }

    private void PopulateRouteMenu(MenuFlyout flyout, DesktopOptionViewModel desktop)
    {
        AddRouteMenuItem(flyout, desktop, null, _resources.GetString("SmartConnectRouteLabel"), true);
        flyout.Items.Add(new MenuFlyoutSeparator());

        foreach (RouteOptionSummary route in desktop.RouteOptions.OrderBy(option => option.Order))
        {
            AddRouteMenuItem(flyout, desktop, route.Adapter, RouteLabel(route.Adapter),
                string.Equals(route.Status, "available", StringComparison.OrdinalIgnoreCase));
        }
    }

	private void DeviceRowGrid_SizeChanged(object sender, SizeChangedEventArgs e)
	{
		if (sender is not Grid row ||
			row.Parent is not Grid card ||
			row.Children.OfType<StackPanel>().FirstOrDefault(child => child.Name == "DeviceIdentity") is not StackPanel identity ||
			row.Children.OfType<StackPanel>().FirstOrDefault(child => child.Name == "DeviceSummary") is not StackPanel summary ||
			row.Children.OfType<Border>().FirstOrDefault(child => child.Name == "DeviceIcon") is not Border deviceIcon ||
			row.FindName("DeviceTitle") is not TextBlock deviceTitle ||
			row.FindName("DeviceConnectionSummary") is not TextBlock connectionSummary ||
			row.Children.OfType<Grid>().FirstOrDefault(child => child.Name == "DeviceActions") is not Grid actions ||
			actions.Children.OfType<SplitButton>().FirstOrDefault() is not SplitButton primaryButton ||
			primaryButton.Content is not StackPanel primaryContent ||
			primaryContent.Children.OfType<FontIcon>().FirstOrDefault() is not FontIcon primaryIcon ||
			primaryContent.Children.OfType<TextBlock>().FirstOrDefault() is not TextBlock primaryLabel ||
			row.DataContext is not DeviceViewModel device ||
			actions.Children.OfType<Button>().FirstOrDefault(child => child.Name == "HandToAgentButton") is not Button agentButton ||
			summary.Children.OfType<TextBlock>().Skip(1).FirstOrDefault() is not TextBlock lastDesktopSummary)
		{
			return;
		}
		bool narrow = e.NewSize.Width < 900;
		bool phone = e.NewSize.Width < 420;
		row.ColumnSpacing = phone ? 0 : 14;
		row.ColumnDefinitions[1].Width = phone ? new GridLength(0) : new GridLength(54);
		row.ColumnDefinitions[2].Width = narrow ? new GridLength(1, GridUnitType.Star) : new GridLength(190);
		row.ColumnDefinitions[3].Width = narrow ? new GridLength(0) : new GridLength(1, GridUnitType.Star);
		row.ColumnDefinitions[4].Width = narrow ? new GridLength(0) : GridLength.Auto;
		deviceIcon.Visibility = phone ? Visibility.Collapsed : Visibility.Visible;
		Grid.SetColumn(identity, phone ? 1 : 2);
		Grid.SetColumnSpan(identity, phone ? 4 : 1);
		identity.Margin = phone ? new Thickness(8, 0, 0, 0) : new Thickness(0);
		Grid.SetRow(summary, narrow ? 1 : 0);
		Grid.SetColumn(summary, phone ? 1 : narrow ? 2 : 3);
		Grid.SetColumnSpan(summary, phone ? 5 : narrow ? 4 : 1);
		summary.Margin = narrow ? new Thickness(phone ? 8 : 0, 8, 0, 0) : new Thickness(0);
		lastDesktopSummary.Visibility = narrow ? Visibility.Collapsed : Visibility.Visible;
		deviceTitle.TextWrapping = narrow ? TextWrapping.Wrap : TextWrapping.NoWrap;
		deviceTitle.TextTrimming = narrow ? TextTrimming.None : TextTrimming.CharacterEllipsis;
		deviceTitle.MaxLines = phone ? 3 : narrow ? 2 : 1;
		connectionSummary.TextWrapping = narrow ? TextWrapping.Wrap : TextWrapping.NoWrap;
		connectionSummary.TextTrimming = narrow ? TextTrimming.None : TextTrimming.CharacterEllipsis;
		connectionSummary.MaxLines = phone ? 3 : narrow ? 2 : 1;
		connectionSummary.Text = phone ? device.CompactConnectionSummaryLabel : device.ConnectionSummaryLabel;
		Grid.SetRow(actions, narrow ? 2 : 0);
		Grid.SetColumn(actions, narrow ? 0 : 4);
		Grid.SetColumnSpan(actions, narrow ? 6 : 1);
		actions.Margin = narrow ? new Thickness(0, 10, 0, 0) : new Thickness(0);
		actions.Width = phone && card.ActualWidth > row.Padding.Left + row.Padding.Right
			? card.ActualWidth - row.Padding.Left - row.Padding.Right
			: double.NaN;
		actions.ColumnDefinitions[0].Width = narrow ? new GridLength(1, GridUnitType.Star) : GridLength.Auto;
		actions.ColumnDefinitions[1].Width = phone ? new GridLength(0) : GridLength.Auto;
		primaryButton.HorizontalAlignment = narrow ? HorizontalAlignment.Stretch : HorizontalAlignment.Left;
		primaryButton.MinWidth = phone ? 0 : 250;
		primaryIcon.Visibility = phone ? Visibility.Collapsed : Visibility.Visible;
		primaryContent.Spacing = phone ? 0 : 8;
		primaryLabel.Text = device.PrimaryDesktopActionLabel;
		primaryLabel.TextWrapping = TextWrapping.Wrap;
		Grid.SetRow(agentButton, phone ? 1 : 0);
		Grid.SetColumn(agentButton, phone ? 0 : 1);
		agentButton.HorizontalAlignment = phone ? HorizontalAlignment.Stretch : HorizontalAlignment.Left;
		agentButton.MinWidth = phone ? 0 : 118;
		agentButton.Margin = phone ? new Thickness(0, 8, 0, 0) : new Thickness(0);
	}

	private void DesktopRowGrid_SizeChanged(object sender, SizeChangedEventArgs e)
	{
		if (sender is not Grid row ||
			row.Children.OfType<StackPanel>().FirstOrDefault(child => child.Name == "DesktopIdentity") is not StackPanel identity ||
			row.Children.OfType<TextBlock>().FirstOrDefault(child => child.Name == "DesktopRecent") is not TextBlock recent ||
			row.Children.OfType<SplitButton>().FirstOrDefault(child => child.Name == "DesktopAction") is not SplitButton action)
		{
			return;
		}

		bool narrow = e.NewSize.Width < 900;
		row.ColumnDefinitions[1].Width = narrow ? new GridLength(1, GridUnitType.Star) : new GridLength(250);
		row.ColumnDefinitions[2].Width = narrow ? new GridLength(0) : new GridLength(1, GridUnitType.Star);
		Grid.SetColumnSpan(identity, narrow ? 3 : 1);
		recent.Visibility = narrow ? Visibility.Collapsed : Visibility.Visible;
		Grid.SetRow(action, narrow ? 1 : 0);
		Grid.SetColumn(action, narrow ? 1 : 3);
		Grid.SetColumnSpan(action, narrow ? 3 : 1);
		action.HorizontalAlignment = narrow ? HorizontalAlignment.Left : HorizontalAlignment.Stretch;
		action.Margin = narrow ? new Thickness(0, 6, 0, 0) : new Thickness(0);
		row.MinHeight = narrow ? 92 : 54;
	}

	private void DeviceDetailsToggle_Toggled(object sender, RoutedEventArgs e)
	{
		if (sender is not ToggleButton toggle ||
			toggle.Parent is not Grid row ||
			row.Parent is not Grid card ||
			card.FindName("DeviceDetailsBorder") is not Border details)
		{
			return;
		}

		bool expanded = toggle.IsChecked == true;
		if (toggle.Tag is string deviceID)
		{
			if (expanded)
			{
				_expandedDeviceIds.Add(deviceID);
			}
			else
			{
				_expandedDeviceIds.Remove(deviceID);
			}
		}
		details.Visibility = expanded ? Visibility.Visible : Visibility.Collapsed;
		if (toggle.Content is FontIcon icon)
		{
			icon.Glyph = expanded ? "\uE70E" : "\uE70D";
		}
	}

	private void ComputersToolbar_SizeChanged(object sender, SizeChangedEventArgs e)
	{
		if (sender is not Grid || ComputerSearchActions is null)
		{
			return;
		}
		bool narrow = e.NewSize.Width < 720;
		Grid.SetRow(ComputerSearchActions, narrow ? 1 : 0);
		Grid.SetColumn(ComputerSearchActions, narrow ? 0 : 1);
		Grid.SetColumnSpan(ComputerSearchActions, narrow ? 2 : 1);
		ComputerSearchActions.Margin = narrow ? new Thickness(0, 10, 0, 0) : new Thickness(0);
		ComputerSearchActions.Width = narrow ? double.NaN : 390;
	}

	private void ComputersHeader_SizeChanged(object sender, SizeChangedEventArgs e)
	{
		bool narrow = e.NewSize.Width < 640;
		Grid.SetRow(AddComputerButton, narrow ? 1 : 0);
		Grid.SetColumn(AddComputerButton, narrow ? 0 : 1);
		Grid.SetColumnSpan(AddComputerButton, narrow ? 2 : 1);
		AddComputerButton.HorizontalAlignment = narrow ? HorizontalAlignment.Stretch : HorizontalAlignment.Right;
		AddComputerButton.Margin = narrow ? new Thickness(0, 14, 0, 0) : new Thickness(0);
	}

	private void SettingsPanel_SizeChanged(object sender, SizeChangedEventArgs e)
	{
		bool compact = e.NewSize.Width < 720;
		if (ReferenceEquals(sender, ConnectionServicePanel))
		{
			ConnectionServicePanel.ColumnDefinitions[1].Width = compact ? new GridLength(0) : GridLength.Auto;
			Grid.SetColumnSpan(ConnectionServiceInfoBar, compact ? 2 : 1);
			Grid.SetRow(ConnectionServiceButton, compact ? 1 : 0);
			Grid.SetColumn(ConnectionServiceButton, compact ? 0 : 1);
			Grid.SetColumnSpan(ConnectionServiceButton, compact ? 2 : 1);
			ConnectionServiceButton.HorizontalAlignment = compact ? HorizontalAlignment.Stretch : HorizontalAlignment.Right;
			ConnectionServiceButton.Margin = compact ? new Thickness(0, 10, 0, 0) : new Thickness(0);
			return;
		}

		if (ReferenceEquals(sender, VersionPanel))
		{
			VersionPanel.Padding = compact ? new Thickness(16, 14, 16, 14) : new Thickness(20, 18, 20, 18);
			ApplySettingsCardLayout(VersionPanelGrid, RollbackVersionButton, compact);
			return;
		}

		if (ReferenceEquals(sender, AgentIntegrationPanel))
		{
			AgentIntegrationPanel.Padding = compact ? new Thickness(16, 14, 16, 14) : new Thickness(20, 18, 20, 18);
			ApplySettingsCardLayout(AgentIntegrationPanelGrid, AgentIntegrationActionsPanel, compact);
			return;
		}

		if (ReferenceEquals(sender, MigrationPanel))
		{
			MigrationPanel.Padding = compact ? new Thickness(16, 14, 16, 14) : new Thickness(20, 18, 20, 18);
			ApplySettingsCardLayout(MigrationPanelGrid, MigrationActionsPanel, compact);
			return;
		}

		if (ReferenceEquals(sender, RecoveryPanel))
		{
			RecoveryPanel.Padding = compact ? new Thickness(16, 14, 16, 14) : new Thickness(20, 18, 20, 18);
			ApplySettingsCardLayout(RecoveryPanelGrid, RecoveryActionsPanel, compact);
			RecoveryActionsPanel.Orientation = compact ? Orientation.Vertical : Orientation.Horizontal;
			CreateRecoveryButton.HorizontalAlignment = compact ? HorizontalAlignment.Stretch : HorizontalAlignment.Left;
			RestoreRecoveryButton.HorizontalAlignment = compact ? HorizontalAlignment.Stretch : HorizontalAlignment.Left;
		}
	}

	private static void ApplySettingsCardLayout(Grid grid, FrameworkElement actions, bool compact)
	{
		grid.ColumnDefinitions[1].Width = compact ? new GridLength(0) : GridLength.Auto;
		Grid.SetRow(actions, compact ? 1 : 0);
		Grid.SetColumn(actions, compact ? 0 : 1);
		Grid.SetColumnSpan(actions, compact ? 2 : 1);
		actions.HorizontalAlignment = compact ? HorizontalAlignment.Stretch : HorizontalAlignment.Right;
		actions.Margin = compact ? new Thickness(0, 16, 0, 0) : new Thickness(0);
	}

	private void RecentSessionGrid_SizeChanged(object sender, SizeChangedEventArgs e)
	{
		if (sender is not Grid grid ||
			grid.Parent is not Border card ||
			grid.Children.OfType<StackPanel>().FirstOrDefault(child => child.Name == "SessionActions") is not StackPanel actions ||
			actions.Children.OfType<Button>().FirstOrDefault() is not Button repeatButton)
		{
			return;
		}

		bool compact = e.NewSize.Width < 600;
		card.Padding = compact ? new Thickness(16, 14, 16, 14) : new Thickness(20, 18, 20, 18);
		grid.ColumnDefinitions[1].Width = compact ? new GridLength(0) : GridLength.Auto;
		Grid.SetRow(actions, compact ? 1 : 0);
		Grid.SetColumn(actions, compact ? 0 : 1);
		Grid.SetColumnSpan(actions, compact ? 2 : 1);
		actions.HorizontalAlignment = compact ? HorizontalAlignment.Stretch : HorizontalAlignment.Right;
		actions.Margin = compact ? new Thickness(0, 12, 0, 0) : new Thickness(0);
		repeatButton.HorizontalAlignment = compact ? HorizontalAlignment.Stretch : HorizontalAlignment.Right;
	}

	private async void RefreshComputersButton_Click(object sender, RoutedEventArgs e)
	{
		await RefreshCatalogAsync(userInitiated: true);
	}

    private void AddRouteMenuItem(MenuFlyout flyout, DesktopOptionViewModel desktop, string? adapter, string label, bool available)
    {
        bool canAttempt = DeviceCatalogPresenter.CanAttemptRoute(desktop.RouteOptions, adapter);
        var item = new MenuFlyoutItem
        {
            Text = string.Format(
                CultureInfo.CurrentCulture,
                _resources.GetString(available ? "RouteAvailableLabel" : canAttempt ? "RouteRetryLabel" : "RouteUnavailableLabel"),
                label),
            IsEnabled = canAttempt,
            Tag = new RouteSelection(desktop.Canonical!, adapter),
        };
        item.Click += RouteMenuItem_Click;
        flyout.Items.Add(item);
    }

    private async void RouteMenuItem_Click(object sender, RoutedEventArgs e)
    {
        if (sender is MenuFlyoutItem { Tag: RouteSelection selection } item)
        {
            await OpenDesktopAsync(selection.Canonical, selection.Adapter, item);
        }
    }

    private async void UseComputerButton_Click(SplitButton sender, SplitButtonClickEventArgs e)
    {
        if (BoundDesktop(sender) is { CanOpen: true, Canonical: string canonical })
        {
            await OpenDesktopAsync(canonical, null, sender);
        }
    }

    private DesktopOptionViewModel? FindDesktop(string canonical) => _catalogFeedback.IsUnavailable ? null : _allDevices
        .SelectMany(device => device.DesktopOptions)
        .FirstOrDefault(option => string.Equals(option.Canonical, canonical, StringComparison.Ordinal));

    private async Task OpenDesktopAsync(string canonical, string? routeAdapter, Control? initiatingControl)
    {
        DesktopOptionViewModel? desktop = FindDesktop(canonical);
        if (desktop is null || !desktop.CanOpen)
        {
            ShowDesktopFailure("TARGET_UNAVAILABLE", routeAdapter, canonical);
            return;
        }
		if (!_desktopOperations.TryBegin(canonical))
		{
			OperationStatus = string.Format(
				CultureInfo.CurrentCulture,
				_resources.GetString("DesktopAlreadyOpening"),
				desktop.ComputerName);
			return;
		}

        if (initiatingControl is not null)
        {
            initiatingControl.IsEnabled = false;
        }
		BeginVisibleOperation();
		_desktopRetryRequest = null;
		RecoverDesktopButton.Visibility = Visibility.Collapsed;
		DesktopActionInfoBar.IsOpen = false;
		AgentHandoffInfoBar.IsOpen = false;
        string routeLabel = RouteLabel(routeAdapter);
        OperationStatus = string.Format(
            CultureInfo.CurrentCulture,
            _resources.GetString(routeAdapter is null ? "DesktopOpening" : "DesktopOpeningVia"),
            desktop.ComputerName,
            routeLabel);
        try
        {
			DesktopActionResponse response = await RunWithDaemonRecoveryAsync(() => _client.OpenAsync(desktop.Canonical!, routeAdapter));
			RecordRecentSession(desktop, response);
            OperationStatus = string.Format(
                CultureInfo.CurrentCulture,
                _resources.GetString(desktop.RequiresFirstUseConfirmation ? "DesktopOpenedFirstUse" : "DesktopOpened"),
				desktop.ComputerName);
        }
        catch (PfRemoteCliException exception) when (exception.Code == "DESKTOP_CREDENTIAL_REQUIRED")
        {
			string? credential = await PromptForDesktopPasswordAsync(desktop.ComputerName);
            if (credential is null)
            {
                OperationStatus = _resources.GetString("DesktopSetupCanceled");
            }
            else
            {
                try
                {
					DesktopActionResponse response = await RunWithDaemonRecoveryAsync(() => _client.SaveDesktopCredentialAndOpenAsync(desktop.Canonical!, credential, routeAdapter));
					RecordRecentSession(desktop, response);
                    OperationStatus = string.Format(
                        CultureInfo.CurrentCulture,
                        _resources.GetString("DesktopCredentialSavedAndOpened"),
						desktop.ComputerName);
                }
                catch (PfRemoteCliException failure)
                {
                    ShowDesktopFailure(failure.Code, routeAdapter, canonical);
                }
                catch (Exception)
                {
                    ShowDesktopFailure(null, routeAdapter, canonical);
                }
            }
        }
        catch (PfRemoteCliException exception)
        {
			ShowDesktopFailure(exception.Code, routeAdapter, canonical);
        }
        catch (Exception)
        {
			ShowDesktopFailure(null, routeAdapter, canonical);
        }
        finally
        {
			_desktopOperations.End(canonical);
			EndVisibleOperation();
            if (initiatingControl is not null)
            {
                initiatingControl.IsEnabled = !_catalogFeedback.IsUnavailable &&
                    (DeviceCatalogPresenter.BoundDesktop(initiatingControl.DataContext)?.CanOpen ?? FindDesktop(canonical)?.CanOpen ?? false);
            }
        }
    }

	private void ShowDesktopFailure(string? code, string? routeAdapter, string? canonical)
	{
		AgentHandoffInfoBar.IsOpen = false;
		DesktopOptionViewModel? desktop = string.IsNullOrWhiteSpace(canonical) ? null : FindDesktop(canonical);
		DesktopFailurePresentation failure = DesktopFailurePresenter.Create(
			code,
			routeAdapter,
			desktop?.ComputerName,
			desktop?.DisplayName,
			_resources.GetString);
		_desktopRetryRequest = string.IsNullOrWhiteSpace(canonical)
			? null
			: new DesktopRetryRequest(canonical, routeAdapter);
		DesktopActionInfoBar.Title = failure.Title;
		DesktopActionInfoBar.Message = failure.Message;
		RecoverDesktopButton.Visibility = _desktopRetryRequest is null ? Visibility.Collapsed : Visibility.Visible;
		if (_desktopRetryRequest is DesktopRetryRequest retry)
		{
			BuildDesktopRecoveryFlyout(retry);
		}
		DesktopActionInfoBar.IsOpen = true;
		OperationStatus = failure.Message;
	}

	private void BuildDesktopRecoveryFlyout(DesktopRetryRequest retry)
	{
		var flyout = new MenuFlyout();
		var retryItem = new MenuFlyoutItem
		{
			Text = _resources.GetString("RetryLastRouteLabel"),
			Tag = new RouteSelection(retry.Canonical, retry.RouteAdapter),
		};
		retryItem.Click += RouteMenuItem_Click;
		flyout.Items.Add(retryItem);

		DesktopOptionViewModel? desktop = FindDesktop(retry.Canonical);
		if (desktop is not null)
		{
			var alternatives = new List<(string? Adapter, string Label)>();
			if (!string.IsNullOrWhiteSpace(retry.RouteAdapter))
			{
				alternatives.Add((null, _resources.GetString("SmartConnectRouteLabel")));
			}
			alternatives.AddRange(desktop.RouteOptions
				.Where(option => string.Equals(option.Status, "available", StringComparison.OrdinalIgnoreCase) &&
					!string.Equals(option.Adapter, retry.RouteAdapter, StringComparison.Ordinal))
				.OrderBy(option => option.Order)
				.Select(option => ((string?)option.Adapter, RouteLabel(option.Adapter))));
			if (alternatives.Count > 0)
			{
				flyout.Items.Add(new MenuFlyoutSeparator());
				foreach ((string? adapter, string label) in alternatives.DistinctBy(item => item.Adapter, StringComparer.Ordinal))
				{
					AddRouteMenuItem(flyout, desktop, adapter, label, true);
				}
			}
		}
		RecoverDesktopButton.Flyout = flyout;
	}

    private string RouteLabel(string? adapter) => adapter switch
    {
        "lan" => _resources.GetString("LanRouteLabel"),
        "tailscale" => _resources.GetString("TailscaleRouteLabel"),
        "frp" => _resources.GetString("GatewayRouteLabel"),
        "legacy-external" => _resources.GetString("ExistingRouteLabel"),
        _ => _resources.GetString("SmartConnectRouteLabel"),
    };

    private sealed record RouteSelection(string Canonical, string? Adapter);
	private sealed record DesktopRetryRequest(string Canonical, string? RouteAdapter);

    private async Task<string?> PromptForDesktopPasswordAsync(string computerName)
    {
        var password = new PasswordBox
        {
            Header = _resources.GetString("DesktopPasswordLabel"),
            PlaceholderText = _resources.GetString("DesktopPasswordPlaceholder"),
        };
        var content = new StackPanel { Spacing = 12, MinWidth = 360 };
        content.Children.Add(new TextBlock
        {
            Text = string.Format(CultureInfo.CurrentCulture, _resources.GetString("DesktopPasswordHelp"), computerName),
            TextWrapping = TextWrapping.Wrap,
        });
        content.Children.Add(password);
        var dialog = new ContentDialog
        {
            XamlRoot = XamlRoot,
            Title = _resources.GetString("DesktopPasswordTitle"),
            Content = content,
            PrimaryButtonText = _resources.GetString("SaveAndOpenButtonLabel"),
            CloseButtonText = _resources.GetString("CancelButtonLabel"),
            DefaultButton = ContentDialogButton.Primary,
        };
        dialog.PrimaryButtonClick += (_, args) =>
        {
            if (string.IsNullOrEmpty(password.Password))
            {
                args.Cancel = true;
            }
        };
        return await ShowDialogAsync(dialog) == ContentDialogResult.Primary ? password.Password : null;
    }

    private async void HandToAgentButton_Click(object sender, RoutedEventArgs e)
    {
        if (sender is not Button { Tag: string canonical } button)
        {
            return;
        }
        DeviceViewModel? device = _allDevices.FirstOrDefault(item =>
            string.Equals(item.AgentCanonical, canonical, StringComparison.Ordinal));
        if (device is null)
        {
            return;
        }
		await HandToAgentAsync(button, device);
	}

    private async Task HandToAgentAsync(Button button, DeviceViewModel device)
	{
        if (_catalogFeedback.IsUnavailable) return;
        button.IsEnabled = false;
        try
        {
			ContextResponse context = await RunWithDaemonRecoveryAsync(() => _client.ContextAsync(device.AgentCanonical));
            if (!ContextHandoffAudit.TryExport(context.Envelope))
            {
                var package = new DataPackage();
                package.SetText(context.Envelope);
                Clipboard.SetContent(package);
                Clipboard.Flush();
            }
			AgentHandoffPresentation handoff = AgentHandoffPresenter.Create(
				_agentIntegrationKind,
				device.Title,
				_resources.GetString);
			OperationStatus = handoff.Status;
			DesktopActionInfoBar.IsOpen = false;
			if (handoff.NoticeTitle is null || handoff.NoticeMessage is null)
			{
				AgentHandoffInfoBar.IsOpen = false;
			}
			else
			{
				AgentHandoffInfoBar.Title = handoff.NoticeTitle;
				AgentHandoffInfoBar.Message = handoff.NoticeMessage;
				AgentHandoffInfoBar.Severity = handoff.IsSuccess ? InfoBarSeverity.Success : InfoBarSeverity.Warning;
				OpenAgentSettingsButton.Visibility = handoff.CanOpenSettings ? Visibility.Visible : Visibility.Collapsed;
				AgentHandoffInfoBar.IsOpen = true;
			}
        }
        catch (Exception)
        {
            OperationStatus = _resources.GetString("ContextCopyFailedStatus");
        }
        finally
        {
            button.IsEnabled = !_catalogFeedback.IsUnavailable && (button.DataContext switch
            {
                DeviceViewModel current => current.CanHandToAgent,
                RecentSessionViewModel current => current.CanRepeat,
                _ => false,
            });
        }
    }

	private void OpenAgentSettingsButton_Click(object sender, RoutedEventArgs e)
	{
		AgentHandoffInfoBar.IsOpen = false;
		RootNavigation.SelectedItem = SettingsNavigationItem;
		ApplyDestination("settings");
		DispatcherQueue.TryEnqueue(() =>
		{
			SettingsScrollViewer.ChangeView(null, 0, null, true);
			AgentIntegrationPanel.StartBringIntoView();
		});
	}

	private async void RepeatSessionButton_Click(object sender, RoutedEventArgs e)
	{
		if (sender is not Button { DataContext: RecentSessionViewModel session } button)
		{
			return;
		}
		RootNavigation.SelectedItem = ComputersNavigationItem;
		ApplyDestination("computers");
		if (string.Equals(session.Action, "open", StringComparison.Ordinal))
		{
			await OpenDesktopAsync(session.CanonicalTarget, null, button);
			return;
		}
		DeviceViewModel? device = _allDevices.FirstOrDefault(item =>
			string.Equals(item.AgentCanonical, session.CanonicalTarget, StringComparison.Ordinal));
		if (device is not null)
		{
			await HandToAgentAsync(button, device);
		}
	}

	private void RootNavigation_SelectionChanged(NavigationView sender, NavigationViewSelectionChangedEventArgs args)
	{
		if (args.SelectedItemContainer?.Tag is string destination)
		{
			ApplyDestination(destination);
			if (destination == "settings")
			{
				DispatcherQueue.TryEnqueue(() => SettingsScrollViewer.ChangeView(null, 0, null, true));
			}
		}
	}

	private void ApplyDestination(string destination)
	{
		bool computers = destination == "computers";
		bool sessions = destination == "sessions";
		bool settings = destination == "settings";
		ComputersHeader.Visibility = computers ? Visibility.Visible : Visibility.Collapsed;
		ComputersToolbar.Visibility = computers ? Visibility.Visible : Visibility.Collapsed;
		AuthorizationInfoBar.Visibility = computers ? Visibility.Visible : Visibility.Collapsed;
        CatalogStatusInfoBar.Visibility = computers ? Visibility.Visible : Visibility.Collapsed;
		DevicesList.Visibility = computers && _catalogStateKnown && Devices.Count > 0 ? Visibility.Visible : Visibility.Collapsed;
		ComputersEmptyState.Visibility = computers && _catalogStateKnown && Devices.Count == 0 ? Visibility.Visible : Visibility.Collapsed;
		SessionsHeader.Visibility = sessions ? Visibility.Visible : Visibility.Collapsed;
		SessionsPanel.Visibility = sessions ? Visibility.Visible : Visibility.Collapsed;
		SettingsScrollViewer.Visibility = settings ? Visibility.Visible : Visibility.Collapsed;
		SettingsHeader.Visibility = settings ? Visibility.Visible : Visibility.Collapsed;
		VersionPanel.Visibility = settings ? Visibility.Visible : Visibility.Collapsed;
		AgentIntegrationPanel.Visibility = settings ? Visibility.Visible : Visibility.Collapsed;
		ConnectionServicePanel.Visibility = settings ? Visibility.Visible : Visibility.Collapsed;
		MigrationPanel.Visibility = settings && _migrationVisibleForSettings ? Visibility.Visible : Visibility.Collapsed;
		RecoveryPanel.Visibility = settings ? Visibility.Visible : Visibility.Collapsed;
		DesktopActionInfoBar.Visibility = computers ? Visibility.Visible : Visibility.Collapsed;
		AgentHandoffInfoBar.Visibility = computers ? Visibility.Visible : Visibility.Collapsed;
		if (!computers)
		{
			StatusTextBlock.Visibility = Visibility.Collapsed;
			if (sessions)
			{
				_ = RefreshCatalogAsync();
			}
		}
		else
		{
			StatusTextBlock.Visibility = Visibility.Visible;
		SetCatalogStatus(_catalogFeedback.IsUnavailable ? _resources.GetString("CatalogUnavailable") : _catalogStateKnown
                ? string.Format(CultureInfo.CurrentCulture, _resources.GetString("CatalogReady"), Devices.Count)
                : _resources.GetString("LoadingStatus/Text"));
		}
		UpdateSessionsVisibility();
	}

	private void RecordRecentSession(DesktopOptionViewModel desktop, DesktopActionResponse response)
	{
		if (RecentSessions.Any(item => string.Equals(item.SessionId, response.SessionId, StringComparison.Ordinal)))
		{
			return;
		}
		RecentSessionViewModel? previous = RecentSessions.FirstOrDefault(item =>
			string.Equals(item.CanonicalTarget, desktop.Canonical, StringComparison.Ordinal) &&
			string.Equals(item.Action, "open", StringComparison.Ordinal));
		if (previous is not null)
		{
			RecentSessions.Remove(previous);
		}
		RecentSessions.Insert(0, new RecentSessionViewModel(
			response.SessionId,
			desktop.Canonical!,
			"open",
			Devices.FirstOrDefault(device => device.DesktopOptions.Any(option =>
				string.Equals(option.Canonical, desktop.Canonical, StringComparison.Ordinal)))?.Title ?? desktop.ComputerName,
			string.Format(CultureInfo.CurrentCulture, _resources.GetString("RecentDesktopAction"), desktop.Label),
			_resources.GetString("RecentSessionOpenedStatus"),
			DateTime.Now.ToString("g", CultureInfo.CurrentCulture),
			_resources.GetString("ReconnectSessionButtonLabel"),
			true));
		while (RecentSessions.Count > 20)
		{
			RecentSessions.RemoveAt(RecentSessions.Count - 1);
		}
		UpdateSessionsVisibility();
	}

	private void SynchronizeRecentSessions(CatalogResponse catalog)
	{
		if (catalog.RecentSessions is null)
		{
			return;
		}
		IReadOnlyList<RecentSessionViewModel> next = RecentSessionPresenter.Create(
			catalog.Targets,
			catalog.RecentSessions,
			DateTimeOffset.Now,
			CultureInfo.CurrentCulture,
			_resources.GetString);
		if (RecentSessions.Count == next.Count &&
			RecentSessions.Select((session, index) => session == next[index]).All(equal => equal))
		{
			return;
		}
		CollectionReconciler.Update(RecentSessions, next, session => session.SessionId, (left, right) => left == right);
		UpdateSessionsVisibility();
	}

	private void UpdateSessionsVisibility()
	{
		bool hasSessions = RecentSessions.Count > 0;
		SessionsEmptyState.Visibility = hasSessions ? Visibility.Collapsed : Visibility.Visible;
		RecentSessionsList.Visibility = hasSessions ? Visibility.Visible : Visibility.Collapsed;
	}

	private void ShowDevelopmentVersion()
	{
		VersionTextBlock.Text = _resources.GetString("VersionDevelopmentText");
		VersionRollbackNote.Text = _resources.GetString("VersionDevelopmentNote");
		RollbackVersionButton.Visibility = Visibility.Collapsed;
	}

	private async Task RefreshInstalledVersionAsync()
	{
		if (!_lifecycleClient.IsAvailable)
		{
			ShowDevelopmentVersion();
			return;
		}
		try
		{
			InstalledVersionResponse version = await _lifecycleClient.StatusAsync();
			VersionTextBlock.Text = string.Format(CultureInfo.CurrentCulture, _resources.GetString("VersionCurrentText"), version.Current);
			if (string.IsNullOrWhiteSpace(version.Previous))
			{
				VersionRollbackNote.Text = _resources.GetString("VersionNoPreviousText");
				RollbackVersionButton.Visibility = Visibility.Collapsed;
			}
			else
			{
				VersionRollbackNote.Text = string.Format(CultureInfo.CurrentCulture, _resources.GetString("VersionPreviousText"), version.Previous);
				RollbackVersionButton.Visibility = Visibility.Visible;
				RollbackVersionButton.IsEnabled = true;
			}
		}
		catch (Exception)
		{
			VersionTextBlock.Text = _resources.GetString("VersionStateUnavailableText");
			VersionRollbackNote.Text = _resources.GetString("VersionStateRecoveryText");
			RollbackVersionButton.Visibility = Visibility.Collapsed;
		}
	}

	private async Task RefreshAgentIntegrationAsync()
	{
		EnableAgentIntegrationButton.IsEnabled = false;
		DisableAgentIntegrationButton.IsEnabled = false;
		try
		{
			AgentIntegrationStatus status = await _agentIntegrationService.GetStatusAsync();
			_agentIntegrationKind = status.Kind;
			switch (status.Kind)
			{
				case AgentIntegrationKind.Ready:
					AgentIntegrationStatusText.Text = _resources.GetString("AgentIntegrationReadyStatus");
					EnableAgentIntegrationButton.Visibility = Visibility.Collapsed;
					DisableAgentIntegrationButton.Visibility = Visibility.Visible;
					DisableAgentIntegrationButton.IsEnabled = true;
					break;
				case AgentIntegrationKind.UpdateAvailable:
					AgentIntegrationStatusText.Text = _resources.GetString("AgentIntegrationUpdateStatus");
					EnableAgentIntegrationButton.Content = _resources.GetString("UpdateAgentIntegrationButtonLabel");
					AutomationProperties.SetName(EnableAgentIntegrationButton, _resources.GetString("UpdateAgentIntegrationButtonAutomationName"));
					EnableAgentIntegrationButton.Visibility = Visibility.Visible;
					EnableAgentIntegrationButton.IsEnabled = true;
					DisableAgentIntegrationButton.Visibility = Visibility.Collapsed;
					break;
				case AgentIntegrationKind.Conflict:
					AgentIntegrationStatusText.Text = _resources.GetString("AgentIntegrationConflictStatus");
					EnableAgentIntegrationButton.Visibility = Visibility.Collapsed;
					DisableAgentIntegrationButton.Visibility = Visibility.Collapsed;
					break;
				case AgentIntegrationKind.Unavailable:
					AgentIntegrationStatusText.Text = _resources.GetString("AgentIntegrationUnavailableStatus");
					EnableAgentIntegrationButton.Visibility = Visibility.Collapsed;
					DisableAgentIntegrationButton.Visibility = Visibility.Collapsed;
					break;
				default:
					AgentIntegrationStatusText.Text = _resources.GetString("AgentIntegrationNotEnabledStatus");
					EnableAgentIntegrationButton.Content = _resources.GetString("EnableAgentIntegrationButtonLabel");
					AutomationProperties.SetName(EnableAgentIntegrationButton, _resources.GetString("EnableAgentIntegrationButtonAutomationName"));
					EnableAgentIntegrationButton.Visibility = Visibility.Visible;
					EnableAgentIntegrationButton.IsEnabled = true;
					DisableAgentIntegrationButton.Visibility = Visibility.Collapsed;
					break;
			}
		}
		catch (Exception)
		{
			_agentIntegrationKind = AgentIntegrationKind.Unavailable;
			AgentIntegrationStatusText.Text = _resources.GetString("AgentIntegrationUnavailableStatus");
			EnableAgentIntegrationButton.Visibility = Visibility.Collapsed;
			DisableAgentIntegrationButton.Visibility = Visibility.Collapsed;
		}
	}

	private async void EnableAgentIntegrationButton_Click(object sender, RoutedEventArgs e)
	{
		EnableAgentIntegrationButton.IsEnabled = false;
		ShowSettingsStatus(_resources.GetString("AgentIntegrationEnablingStatus"), InfoBarSeverity.Informational);
		try
		{
			await _agentIntegrationService.EnableAsync();
			await RefreshAgentIntegrationAsync();
			ShowSettingsStatus(_resources.GetString("AgentIntegrationEnabledStatus"), InfoBarSeverity.Success);
		}
		catch (Exception)
		{
			await RefreshAgentIntegrationAsync();
			ShowSettingsStatus(_resources.GetString("AgentIntegrationEnableFailedStatus"), InfoBarSeverity.Error);
		}
	}

	private async void DisableAgentIntegrationButton_Click(object sender, RoutedEventArgs e)
	{
		DisableAgentIntegrationButton.IsEnabled = false;
		ShowSettingsStatus(_resources.GetString("AgentIntegrationDisablingStatus"), InfoBarSeverity.Informational);
		try
		{
			await _agentIntegrationService.DisableAsync();
			await RefreshAgentIntegrationAsync();
			ShowSettingsStatus(_resources.GetString("AgentIntegrationDisabledStatus"), InfoBarSeverity.Success);
		}
		catch (Exception)
		{
			await RefreshAgentIntegrationAsync();
			ShowSettingsStatus(_resources.GetString("AgentIntegrationDisableFailedStatus"), InfoBarSeverity.Error);
		}
	}

	private async void RollbackVersionButton_Click(object sender, RoutedEventArgs e)
	{
		var dialog = new ContentDialog
		{
			Title = _resources.GetString("RollbackConfirmTitle"),
			Content = _resources.GetString("RollbackConfirmMessage"),
			PrimaryButtonText = _resources.GetString("RollbackConfirmButton"),
			CloseButtonText = _resources.GetString("CancelButtonLabel"),
			DefaultButton = ContentDialogButton.Close,
			XamlRoot = XamlRoot,
		};
		if (await ShowDialogAsync(dialog) != ContentDialogResult.Primary)
		{
			return;
		}
		RollbackVersionButton.IsEnabled = false;
		ShowSettingsStatus(_resources.GetString("RollbackStartingStatus"), InfoBarSeverity.Informational);
		try
		{
			InstalledVersionResponse result = await _lifecycleClient.RollbackAsync();
			var startInfo = new System.Diagnostics.ProcessStartInfo
			{
				FileName = result.AppPath,
				WorkingDirectory = Path.GetDirectoryName(result.AppPath) ?? AppContext.BaseDirectory,
				UseShellExecute = false,
			};
			_ = System.Diagnostics.Process.Start(startInfo);
			Application.Current.Exit();
		}
		catch (Exception)
		{
			ShowSettingsStatus(_resources.GetString("RollbackFailedStatus"), InfoBarSeverity.Error);
			RollbackVersionButton.IsEnabled = true;
			await RefreshInstalledVersionAsync();
		}
	}

	private async void CreateRecoveryButton_Click(object sender, RoutedEventArgs e)
	{
		string? password = await PromptForRecoveryPasswordAsync(confirm: true);
		if (password is null)
		{
			return;
		}
		var picker = new FileSavePicker
		{
			SuggestedStartLocation = PickerLocationId.DocumentsLibrary,
			SuggestedFileName = $"PF-Remote-Recovery-{DateTime.Now:yyyy-MM-dd}",
		};
		picker.FileTypeChoices.Add(_resources.GetString("RecoveryFileType"), [".pfremote-recovery"]);
		InitializePicker(picker);
		StorageFile? file = await picker.PickSaveFileAsync();
		if (file is null)
		{
			return;
		}
		CreateRecoveryButton.IsEnabled = false;
		ShowSettingsStatus(_resources.GetString("RecoveryCreatingStatus"), InfoBarSeverity.Informational);
		try
		{
			byte[] recoveryFile = await _recoveryClient.ExportAsync(password);
			await FileIO.WriteBytesAsync(file, recoveryFile);
			ShowSettingsStatus(_resources.GetString("RecoveryCreatedStatus"), InfoBarSeverity.Success);
		}
		catch (Exception)
		{
			ShowSettingsStatus(_resources.GetString("RecoveryCreateFailedStatus"), InfoBarSeverity.Error);
		}
		finally
		{
			CreateRecoveryButton.IsEnabled = true;
		}
	}

	private async void RestoreRecoveryButton_Click(object sender, RoutedEventArgs e)
	{
		var picker = new FileOpenPicker { SuggestedStartLocation = PickerLocationId.DocumentsLibrary };
		picker.FileTypeFilter.Add(".pfremote-recovery");
		InitializePicker(picker);
		StorageFile? file = await picker.PickSingleFileAsync();
		if (file is null)
		{
			return;
		}
		string? password = await PromptForRecoveryPasswordAsync(confirm: false);
		if (password is null)
		{
			return;
		}
		RestoreRecoveryButton.IsEnabled = false;
		ShowSettingsStatus(_resources.GetString("RecoveryRestoringStatus"), InfoBarSeverity.Informational);
		try
		{
			await _recoveryClient.RestoreAsync(file.Path, password);
			ShowSettingsStatus(_resources.GetString("RecoveryRestoredStatus"), InfoBarSeverity.Success);
		}
		catch (Exception)
		{
			ShowSettingsStatus(_resources.GetString("RecoveryRestoreFailedStatus"), InfoBarSeverity.Error);
		}
		finally
		{
			RestoreRecoveryButton.IsEnabled = true;
		}
	}

	private async Task<string?> PromptForRecoveryPasswordAsync(bool confirm)
	{
		var password = new PasswordBox { Header = _resources.GetString("RecoveryPasswordLabel") };
		var confirmation = new PasswordBox
		{
			Header = _resources.GetString("RecoveryPasswordConfirmLabel"),
			Visibility = confirm ? Visibility.Visible : Visibility.Collapsed,
		};
		var error = new TextBlock
		{
			Foreground = (Microsoft.UI.Xaml.Media.Brush)Application.Current.Resources["SystemFillColorCriticalBrush"],
			TextWrapping = TextWrapping.Wrap,
			Visibility = Visibility.Collapsed,
		};
		var content = new StackPanel { Spacing = 12, MinWidth = 360 };
		content.Children.Add(new TextBlock
		{
			Text = _resources.GetString(confirm ? "RecoveryPasswordCreateHelp" : "RecoveryPasswordRestoreHelp"),
			TextWrapping = TextWrapping.Wrap,
		});
		content.Children.Add(password);
		content.Children.Add(confirmation);
		content.Children.Add(error);
		var dialog = new ContentDialog
		{
			XamlRoot = XamlRoot,
			Title = _resources.GetString(confirm ? "RecoveryPasswordCreateTitle" : "RecoveryPasswordRestoreTitle"),
			Content = content,
			PrimaryButtonText = _resources.GetString("ContinueButtonLabel"),
			CloseButtonText = _resources.GetString("CancelButtonLabel"),
			DefaultButton = ContentDialogButton.Primary,
		};
		dialog.PrimaryButtonClick += (_, args) =>
		{
			bool valid = password.Password.Trim().Length >= 12 && (!confirm || password.Password == confirmation.Password);
			if (!valid)
			{
				args.Cancel = true;
				error.Text = _resources.GetString(confirm ? "RecoveryPasswordValidation" : "RecoveryPasswordRequired");
				error.Visibility = Visibility.Visible;
			}
		};
		return await ShowDialogAsync(dialog) == ContentDialogResult.Primary ? password.Password : null;
	}

	private static void InitializePicker(object picker)
	{
		Window? window = (Application.Current as App)?.ActiveWindow;
		if (window is null)
		{
			throw new InvalidOperationException("PF Remote window is unavailable.");
		}
		IntPtr handle = WinRT.Interop.WindowNative.GetWindowHandle(window);
		WinRT.Interop.InitializeWithWindow.Initialize(picker, handle);
	}

}
