using System.Text;
using System.Text.Json;
using System.Windows.Automation;

namespace GoldenJourneyUI;

internal static class Program
{
    private static readonly JsonSerializerOptions JsonOptions = new() { WriteIndented = true };

    private static int Main()
    {
        string outputPath = Required("PFREMOTE_GOLDEN_UI_RESULT_PATH");
        try
        {
            int processId = int.Parse(Required("PFREMOTE_GOLDEN_CENTER_PID"), System.Globalization.CultureInfo.InvariantCulture);
            AutomationElement root = WaitForWindow(processId, TimeSpan.FromSeconds(20));
            if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "lifecycle", StringComparison.OrdinalIgnoreCase))
            {
                return RunLifecycle(root, outputPath);
            }
            if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "handoff", StringComparison.OrdinalIgnoreCase))
            {
                return RunHandoff(root, outputPath);
            }
			if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "routes", StringComparison.OrdinalIgnoreCase))
			{
				return RunRoutes(root, outputPath);
			}
			if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "route-launch", StringComparison.OrdinalIgnoreCase))
			{
				return RunRouteLaunch(root, outputPath);
			}
			if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "refresh", StringComparison.OrdinalIgnoreCase))
			{
				return RunRefresh(root, outputPath);
			}
			if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "stable-refresh", StringComparison.OrdinalIgnoreCase))
			{
				return RunStableRefresh(root, outputPath);
			}
			if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "startup", StringComparison.OrdinalIgnoreCase))
			{
				return RunStartup(root, outputPath);
			}
			if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "desktop-launch", StringComparison.OrdinalIgnoreCase))
			{
				return RunDesktopLaunch(root, outputPath);
			}
			if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "retry-desktop", StringComparison.OrdinalIgnoreCase))
			{
				return RunRetryDesktop(root, outputPath);
			}
			if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "agent-integration", StringComparison.OrdinalIgnoreCase))
			{
				return RunAgentIntegration(root, outputPath);
			}
			if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "agent-handoff-notice", StringComparison.OrdinalIgnoreCase))
			{
				return RunAgentHandoffNotice(root, outputPath);
			}
			if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "agent-handoff-ready", StringComparison.OrdinalIgnoreCase))
			{
				return RunAgentHandoffReady(root, outputPath);
			}
			if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "target-clarity", StringComparison.OrdinalIgnoreCase))
			{
				return RunTargetClarity(root, outputPath);
			}
			if (string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE"), "recovery-menu", StringComparison.OrdinalIgnoreCase))
			{
				return RunRecoveryMenu(root, outputPath);
			}
            AutomationElement desktop = WaitForDesktopAction(root, TimeSpan.FromSeconds(15));
            bool desktopKeyboardFocusable = desktop.Current.IsKeyboardFocusable;
            Invoke(desktop);
            WaitForFile(Required("PFREMOTE_GOLDEN_DESKTOP_RECORD_PATH"), TimeSpan.FromSeconds(10));
            string desktopStatus = CurrentStatus(root);

            AutomationElement agent = WaitForElement(root, Required("PFREMOTE_GOLDEN_AGENT_AUTOMATION_ID"), TimeSpan.FromSeconds(15));
            bool agentKeyboardFocusable = agent.Current.IsKeyboardFocusable;
            Invoke(agent);
            WaitForFile(Required("PFREMOTE_GOLDEN_CONTEXT_PATH"), TimeSpan.FromSeconds(10));
            string agentStatus = CurrentStatus(root);

			OpenSessions(root);
			AutomationElement repeatAction = WaitForVisibleElement(
				root,
				Required("PFREMOTE_GOLDEN_REPEAT_SESSION_AUTOMATION_ID"),
				TimeSpan.FromSeconds(15));

            WriteResult(outputPath, new
            {
                schema_version = "pfremote.golden-ui-automation/v1",
                status = "passed",
                desktop_button_name = desktop.Current.Name,
                agent_button_name = agent.Current.Name,
                desktop_keyboard_focusable = desktopKeyboardFocusable,
                agent_keyboard_focusable = agentKeyboardFocusable,
                desktop_status = desktopStatus,
                agent_status = agentStatus,
				repeat_session_visible = !repeatAction.Current.IsOffscreen,
                input_injection_used = false,
            });
            return 0;
        }
        catch (Exception failure)
        {
            WriteResult(outputPath, new
            {
                schema_version = "pfremote.golden-ui-automation/v1",
                status = "failed",
                error = failure.Message,
				mode = Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_MODE") ?? "",
                input_injection_used = false,
            });
            return 1;
        }
    }

	private static int RunRoutes(AutomationElement root, string outputPath)
	{
		AutomationElement split = WaitForDesktopAction(root, TimeSpan.FromSeconds(15));
		if (split.TryGetCurrentPattern(ExpandCollapsePattern.Pattern, out object expandPattern))
		{
			((ExpandCollapsePattern)expandPattern).Expand();
		}
		else
		{
			AutomationElement? expander = split.FindAll(TreeScope.Descendants, new PropertyCondition(AutomationElement.ControlTypeProperty, ControlType.Button))
				.Cast<AutomationElement>()
				.FirstOrDefault(candidate => candidate.TryGetCurrentPattern(ExpandCollapsePattern.Pattern, out _));
			if (expander is null || !expander.TryGetCurrentPattern(ExpandCollapsePattern.Pattern, out expandPattern))
			{
				throw new InvalidOperationException("The per-connection route menu is not keyboard-automation accessible.");
			}
			((ExpandCollapsePattern)expandPattern).Expand();
		}

		string[] expectedNames =
		[
			Required("PFREMOTE_GOLDEN_ROUTE_SMART_NAME"),
			Required("PFREMOTE_GOLDEN_ROUTE_LAN_NAME"),
			Required("PFREMOTE_GOLDEN_ROUTE_TAILSCALE_NAME"),
			Required("PFREMOTE_GOLDEN_ROUTE_GATEWAY_NAME"),
		];
		var items = expectedNames.Select(name => WaitForNamedElementAnyState(name, ControlType.MenuItem, TimeSpan.FromSeconds(10))).ToArray();
		WriteResult(outputPath, new
		{
			schema_version = "pfremote.route-menu-ui-automation/v1",
			status = "passed",
			split_button_name = split.Current.Name,
			split_button_keyboard_focusable = split.Current.IsKeyboardFocusable,
			routes = items.Select(item => new { name = item.Current.Name, enabled = item.Current.IsEnabled }).ToArray(),
			input_injection_used = false,
		});
		return 0;
	}

	private static int RunRouteLaunch(AutomationElement root, string outputPath)
	{
		AutomationElement split = WaitForDesktopAction(root, TimeSpan.FromSeconds(15));
		if (!split.TryGetCurrentPattern(ExpandCollapsePattern.Pattern, out object expandPattern))
		{
			AutomationElement? expander = split.FindAll(TreeScope.Descendants, new PropertyCondition(AutomationElement.ControlTypeProperty, ControlType.Button))
				.Cast<AutomationElement>()
				.FirstOrDefault(candidate => candidate.TryGetCurrentPattern(ExpandCollapsePattern.Pattern, out _));
			if (expander is null || !expander.TryGetCurrentPattern(ExpandCollapsePattern.Pattern, out expandPattern))
			{
				throw new InvalidOperationException("The per-connection route menu is not keyboard-automation accessible.");
			}
		}
		((ExpandCollapsePattern)expandPattern).Expand();

		AutomationElement route = WaitForNamedElement(
			Required("PFREMOTE_GOLDEN_ROUTE_ACTION_NAME"),
			ControlType.MenuItem,
			TimeSpan.FromSeconds(10));
		var existingProcesses = System.Diagnostics.Process.GetProcesses().Select(process => process.Id).ToHashSet();
		var visibleWatch = System.Diagnostics.Stopwatch.StartNew();
		Invoke(route);
		(string processName, string windowName) = WaitForDesktopExecutorWindow(existingProcesses, TimeSpan.FromSeconds(20));
		visibleWatch.Stop();

		WriteResult(outputPath, new
		{
			schema_version = "pfremote.route-launch-ui-automation/v1",
			status = "passed",
			route_name = route.Current.Name,
			route_keyboard_focusable = route.Current.IsKeyboardFocusable,
			executor_process = processName,
			executor_window_visible = true,
			executor_window_name_present = !string.IsNullOrWhiteSpace(windowName),
			post_invoke_visible_seconds = Math.Round(visibleWatch.Elapsed.TotalSeconds, 3),
			center_status = CurrentStatus(root),
			input_injection_used = false,
		});
		CloseCenterWindow();
		return 0;
	}

	private static int RunTargetClarity(AutomationElement root, string outputPath)
	{
		AutomationElement action = WaitForVisibleElement(
			root,
			Required("PFREMOTE_GOLDEN_PRIMARY_DESKTOP_AUTOMATION_ID"),
			TimeSpan.FromSeconds(20));
		string name = action.Current.Name;
		string expectedComputer = Required("PFREMOTE_GOLDEN_EXPECTED_COMPUTER_NAME");
		string expectedDesktop = Required("PFREMOTE_GOLDEN_EXPECTED_DESKTOP_NAME");
		if (!name.Contains(expectedComputer, StringComparison.CurrentCultureIgnoreCase) ||
			!name.Contains(expectedDesktop, StringComparison.CurrentCultureIgnoreCase))
		{
			throw new InvalidOperationException("The primary connection action does not identify both the computer and desktop.");
		}

		WriteResult(outputPath, new
		{
			schema_version = "pfremote.target-clarity-ui-automation/v1",
			status = "passed",
			primary_action_name = name,
			computer_name_present = true,
			desktop_name_present = true,
			primary_action_keyboard_focusable = action.Current.IsKeyboardFocusable,
			input_injection_used = false,
		});
		CloseCenterWindow();
		return 0;
	}

	private static int RunRecoveryMenu(AutomationElement root, string outputPath)
	{
		AutomationElement recovery = WaitForVisibleElement(root, "RetryDesktopButton", TimeSpan.FromSeconds(15));
		if (!recovery.TryGetCurrentPattern(ExpandCollapsePattern.Pattern, out object expandPattern))
		{
			throw new InvalidOperationException("The recovery menu is not keyboard-automation accessible.");
		}
		((ExpandCollapsePattern)expandPattern).Expand();
		string[] expectedNames =
		[
			Required("PFREMOTE_GOLDEN_RECOVERY_RETRY_NAME"),
			Required("PFREMOTE_GOLDEN_RECOVERY_ROUTE_ONE"),
			Required("PFREMOTE_GOLDEN_RECOVERY_ROUTE_TWO"),
		];
		AutomationElement[] items = expectedNames
			.Select(name => WaitForNamedElement(name, ControlType.MenuItem, TimeSpan.FromSeconds(10)))
			.ToArray();

		WriteResult(outputPath, new
		{
			schema_version = "pfremote.recovery-menu-ui-automation/v1",
			status = "passed",
			recovery_button_name = recovery.Current.Name,
			recovery_button_keyboard_focusable = recovery.Current.IsKeyboardFocusable,
			choices = items.Select(item => new { name = item.Current.Name, enabled = item.Current.IsEnabled }).ToArray(),
			input_injection_used = false,
		});
		CloseCenterWindow();
		return 0;
	}

	private static int RunDesktopLaunch(AutomationElement root, string outputPath)
	{
		AutomationElement desktop = WaitForDesktopAction(root, TimeSpan.FromSeconds(20));
		bool keyboardFocusable = desktop.Current.IsKeyboardFocusable;
		var existingProcesses = System.Diagnostics.Process.GetProcesses().Select(process => process.Id).ToHashSet();
		var totalWatch = System.Diagnostics.Stopwatch.StartNew();
		Invoke(desktop);
		var visibleWatch = System.Diagnostics.Stopwatch.StartNew();
		(string processName, string windowName) = WaitForDesktopExecutorWindow(existingProcesses, TimeSpan.FromSeconds(20));
		visibleWatch.Stop();
		totalWatch.Stop();

		WriteResult(outputPath, new
		{
			schema_version = "pfremote.desktop-launch-ui-automation/v1",
			status = "passed",
			desktop_button_name = desktop.Current.Name,
			desktop_keyboard_focusable = keyboardFocusable,
			executor_process = processName,
			executor_window_visible = true,
			executor_window_name_present = !string.IsNullOrWhiteSpace(windowName),
			automation_total_seconds = Math.Round(totalWatch.Elapsed.TotalSeconds, 3),
			post_invoke_visible_seconds = Math.Round(visibleWatch.Elapsed.TotalSeconds, 3),
			center_status = CurrentStatus(root),
			input_injection_used = false,
		});
		CloseCenterWindow();
		return 0;
	}

	private static int RunRetryDesktop(AutomationElement root, string outputPath)
	{
		AutomationElement retry = WaitForElement(root, "RetryDesktopButton", TimeSpan.FromSeconds(15));
		bool keyboardFocusable = retry.Current.IsKeyboardFocusable;
		var existingProcesses = System.Diagnostics.Process.GetProcesses().Select(process => process.Id).ToHashSet();
		var visibleWatch = System.Diagnostics.Stopwatch.StartNew();
		Invoke(retry);
		(string processName, string windowName) = WaitForDesktopExecutorWindow(existingProcesses, TimeSpan.FromSeconds(20));
		visibleWatch.Stop();

		WriteResult(outputPath, new
		{
			schema_version = "pfremote.desktop-retry-ui-automation/v1",
			status = "passed",
			retry_button_name = retry.Current.Name,
			retry_keyboard_focusable = keyboardFocusable,
			executor_process = processName,
			executor_window_visible = true,
			executor_window_name_present = !string.IsNullOrWhiteSpace(windowName),
			post_invoke_visible_seconds = Math.Round(visibleWatch.Elapsed.TotalSeconds, 3),
			center_status = CurrentStatus(root),
			input_injection_used = false,
		});
		return 0;
	}

	private static int RunStartup(AutomationElement root, string outputPath)
	{
		DateTimeOffset started = DateTimeOffset.Parse(
			Required("PFREMOTE_GOLDEN_LAUNCH_STARTED_UTC"),
			System.Globalization.CultureInfo.InvariantCulture,
			System.Globalization.DateTimeStyles.RoundtripKind);
		string? exactReadyStatus = Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_READY_STATUS");
		string readyStatus = string.IsNullOrWhiteSpace(exactReadyStatus)
			? WaitForStatusPrefix(root, Required("PFREMOTE_GOLDEN_READY_STATUS_PREFIX"), TimeSpan.FromSeconds(20))
			: WaitForStatus(root, exactReadyStatus, TimeSpan.FromSeconds(20));
		double readySeconds = (DateTimeOffset.UtcNow - started).TotalSeconds;
		AutomationElement refresh = WaitForElement(root, "RefreshComputersButton", TimeSpan.FromSeconds(5));

		WriteResult(outputPath, new
		{
			schema_version = "pfremote.startup-ui-automation/v1",
			status = "passed",
			ready_status = readyStatus,
			catalog_ready_seconds = Math.Round(readySeconds, 3),
			refresh_keyboard_focusable = refresh.Current.IsKeyboardFocusable,
			input_injection_used = false,
		});
		CloseCenterWindow();
		return 0;
	}

	private static int RunAgentIntegration(AutomationElement root, string outputPath)
	{
		OpenSettings(root);
		bool disable = string.Equals(
			Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_AGENT_INTEGRATION_ACTION"),
			"disable",
			StringComparison.OrdinalIgnoreCase);
		string actionId = disable ? "DisableAgentIntegrationButton" : "EnableAgentIntegrationButton";
		string resultId = disable ? "EnableAgentIntegrationButton" : "DisableAgentIntegrationButton";
		AutomationElement action = WaitForVisibleElement(root, actionId, TimeSpan.FromSeconds(20));
		string actionName = action.Current.Name;
		bool keyboardFocusable = action.Current.IsKeyboardFocusable;
		Invoke(action);
		AutomationElement resultingAction = WaitForVisibleElement(root, resultId, TimeSpan.FromSeconds(30));
		AutomationElement status = WaitForVisibleElement(root, "AgentIntegrationStatus", TimeSpan.FromSeconds(10));

		WriteResult(outputPath, new
		{
			schema_version = "pfremote.agent-integration-ui-automation/v1",
			status = "passed",
			action = disable ? "disable" : "enable",
			action_name = actionName,
			action_keyboard_focusable = keyboardFocusable,
			resulting_action_visible = !resultingAction.Current.IsOffscreen,
			integration_status = status.Current.Name,
			input_injection_used = false,
		});
		CloseCenterWindow();
		return 0;
	}

	private static int RunAgentHandoffNotice(AutomationElement root, string outputPath)
	{
		AutomationElement handoff = WaitForElement(root, Required("PFREMOTE_GOLDEN_AGENT_AUTOMATION_ID"), TimeSpan.FromSeconds(20));
		Invoke(handoff);
		AutomationElement settingsAction = WaitForVisibleElement(root, "OpenAgentSettingsButton", TimeSpan.FromSeconds(20));
		AutomationElement? notice = root.FindFirst(
			TreeScope.Descendants,
			new PropertyCondition(AutomationElement.AutomationIdProperty, "AgentHandoffNotice"));
		string noticeName = notice?.Current.Name ?? string.Empty;
		string actionName = settingsAction.Current.Name;
		bool actionKeyboardFocusable = settingsAction.Current.IsKeyboardFocusable;
		Invoke(settingsAction);
		AutomationElement integrationStatus = WaitForVisibleElement(root, "AgentIntegrationStatus", TimeSpan.FromSeconds(15));

		WriteResult(outputPath, new
		{
			schema_version = "pfremote.agent-handoff-notice-ui-automation/v1",
			status = "passed",
			notice_name = noticeName,
			settings_action_name = actionName,
			settings_action_keyboard_focusable = actionKeyboardFocusable,
			integration_status_visible = !integrationStatus.Current.IsOffscreen,
			input_injection_used = false,
		});
		CloseCenterWindow();
		return 0;
	}

	private static int RunAgentHandoffReady(AutomationElement root, string outputPath)
	{
		AutomationElement handoff = WaitForElement(root, Required("PFREMOTE_GOLDEN_AGENT_AUTOMATION_ID"), TimeSpan.FromSeconds(20));
		bool keyboardFocusable = handoff.Current.IsKeyboardFocusable;
		Invoke(handoff);
		string status = WaitForStatus(root, Required("PFREMOTE_GOLDEN_READY_HANDOFF_STATUS"), TimeSpan.FromSeconds(20));
		AutomationElement notice = WaitForVisibleElement(root, "AgentHandoffNotice", TimeSpan.FromSeconds(10));
		AutomationElement? settingsAction = root.FindFirst(
			TreeScope.Descendants,
			new PropertyCondition(AutomationElement.AutomationIdProperty, "OpenAgentSettingsButton"));
		bool settingsActionVisible = settingsAction is not null && !settingsAction.Current.IsOffscreen;
		if (settingsActionVisible)
		{
			throw new InvalidOperationException("The ready handoff should not offer Codex setup again.");
		}

		WriteResult(outputPath, new
		{
			schema_version = "pfremote.agent-handoff-ready-ui-automation/v1",
			status = "passed",
			handoff_button_name = handoff.Current.Name,
			handoff_keyboard_focusable = keyboardFocusable,
			ready_status = status,
			notice_name = notice.Current.Name,
			settings_action_visible = settingsActionVisible,
			input_injection_used = false,
		});
		CloseCenterWindow();
		return 0;
	}

	private static void CloseCenterWindow()
	{
		int processId = int.Parse(
			Required("PFREMOTE_GOLDEN_CENTER_PID"),
			System.Globalization.CultureInfo.InvariantCulture);
		using System.Diagnostics.Process process = System.Diagnostics.Process.GetProcessById(processId);
		if (!process.CloseMainWindow())
		{
			throw new InvalidOperationException("PF Remote Center did not accept the isolated close request.");
		}
		if (!process.WaitForExit(3000))
		{
			process.Kill(entireProcessTree: true);
			process.WaitForExit(3000);
		}
	}

	private static int RunRefresh(AutomationElement root, string outputPath)
	{
		int cycles = int.TryParse(Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_REFRESH_COUNT"), out int requestedCycles)
			? Math.Clamp(requestedCycles, 1, 50)
			: 1;
		bool keyboardFocusable = false;
		bool disabledWhileRefreshing = true;
		string refreshingStatus = "";
		string readyStatus = "";
		double observedActiveSeconds = 0;
		var refreshWatch = System.Diagnostics.Stopwatch.StartNew();
		for (int cycle = 0; cycle < cycles; cycle++)
		{
			AutomationElement refresh = WaitForElement(root, "RefreshComputersButton", TimeSpan.FromSeconds(15));
			keyboardFocusable |= refresh.Current.IsKeyboardFocusable;
			Invoke(refresh);
			refreshingStatus = WaitForStatus(root, Required("PFREMOTE_GOLDEN_REFRESHING_STATUS"), TimeSpan.FromSeconds(5));
			disabledWhileRefreshing &= !refresh.Current.IsEnabled;
			var activeWatch = System.Diagnostics.Stopwatch.StartNew();
			readyStatus = WaitForStatusPrefix(root, Required("PFREMOTE_GOLDEN_READY_STATUS_PREFIX"), TimeSpan.FromSeconds(15));
			activeWatch.Stop();
			observedActiveSeconds += activeWatch.Elapsed.TotalSeconds;
			_ = WaitForElement(root, "RefreshComputersButton", TimeSpan.FromSeconds(5));
		}
		refreshWatch.Stop();

		if (!disabledWhileRefreshing)
		{
			throw new InvalidOperationException("Refresh remained enabled while at least one catalog operation was active.");
		}

		WriteResult(outputPath, new
		{
			schema_version = "pfremote.refresh-ui-automation/v1",
			status = "passed",
			refreshing_status = refreshingStatus,
			ready_status = readyStatus,
			refresh_disabled_while_active = disabledWhileRefreshing,
			refresh_enabled_after_completion = true,
			refresh_keyboard_focusable = keyboardFocusable,
			cycles_completed = cycles,
			automation_elapsed_seconds = Math.Round(refreshWatch.Elapsed.TotalSeconds, 3),
			automation_cycle_average_seconds = Math.Round(refreshWatch.Elapsed.TotalSeconds / cycles, 3),
			observed_active_average_seconds = Math.Round(observedActiveSeconds / cycles, 3),
			input_injection_used = false,
		});
		return 0;
	}

	private static int RunStableRefresh(AutomationElement root, string outputPath)
	{
		string detailsID = Required("PFREMOTE_GOLDEN_DETAILS_AUTOMATION_ID");
		AutomationElement details = WaitForVisibleElement(root, detailsID, TimeSpan.FromSeconds(15));
		if (!details.TryGetCurrentPattern(TogglePattern.Pattern, out object togglePattern))
		{
			throw new InvalidOperationException("The computer details control does not expose toggle state.");
		}
		if (((TogglePattern)togglePattern).Current.ToggleState != ToggleState.On)
		{
			((TogglePattern)togglePattern).Toggle();
		}
		int[] originalRuntimeID = details.GetRuntimeId();
		bool remainedExpanded = true;
		bool retainedContainer = true;
		for (int cycle = 0; cycle < 3; cycle++)
		{
			AutomationElement refresh = WaitForElement(root, "RefreshComputersButton", TimeSpan.FromSeconds(10));
			Invoke(refresh);
			_ = WaitForStatus(root, Required("PFREMOTE_GOLDEN_REFRESHING_STATUS"), TimeSpan.FromSeconds(5));
			_ = WaitForStatusPrefix(root, Required("PFREMOTE_GOLDEN_READY_STATUS_PREFIX"), TimeSpan.FromSeconds(15));
			details = WaitForVisibleElement(root, detailsID, TimeSpan.FromSeconds(10));
			if (!details.TryGetCurrentPattern(TogglePattern.Pattern, out togglePattern) ||
				((TogglePattern)togglePattern).Current.ToggleState != ToggleState.On)
			{
				remainedExpanded = false;
			}
			retainedContainer &= originalRuntimeID.SequenceEqual(details.GetRuntimeId());
		}
		if (!remainedExpanded || !retainedContainer)
		{
			throw new InvalidOperationException("Catalog refresh replaced or collapsed the open computer details.");
		}

		WriteResult(outputPath, new
		{
			schema_version = "pfremote.stable-refresh-ui-automation/v1",
			status = "passed",
			cycles_completed = 3,
			details_remained_expanded = remainedExpanded,
			container_retained = retainedContainer,
			input_injection_used = false,
		});
		return 0;
	}

    private static int RunLifecycle(AutomationElement root, string outputPath)
    {
        OpenSettings(root);
        AutomationElement rollback = WaitForElement(root, "RollbackVersionButton", TimeSpan.FromSeconds(15));
        string rollbackName = rollback.Current.Name;
        Invoke(rollback);
        AutomationElement confirmation = WaitForNamedButton(Required("PFREMOTE_GOLDEN_CONFIRM_BUTTON"), TimeSpan.FromSeconds(15));
        string confirmationName = confirmation.Current.Name;
        Invoke(confirmation);
        WriteResult(outputPath, new
        {
            schema_version = "pfremote.lifecycle-ui-automation/v1",
            status = "passed",
            rollback_button_name = rollbackName,
            confirmation_button_name = confirmationName,
            input_injection_used = false,
        });
        return 0;
    }

    private static int RunHandoff(AutomationElement root, string outputPath)
    {
        AutomationElement agent = WaitForElement(root, Required("PFREMOTE_GOLDEN_AGENT_AUTOMATION_ID"), TimeSpan.FromSeconds(20));
        string agentName = agent.Current.Name;
        Invoke(agent);
        WaitForFile(Required("PFREMOTE_GOLDEN_CONTEXT_PATH"), TimeSpan.FromSeconds(15));
		WaitForFile(Required("PFREMOTE_GOLDEN_ACTION_READY_PATH"), TimeSpan.FromSeconds(45));

        OpenSessions(root);
        string computerName = Required("PFREMOTE_GOLDEN_SESSION_COMPUTER_NAME");
        string actionName = Required("PFREMOTE_GOLDEN_SESSION_ACTION_NAME");
        AutomationElement recentSessions = WaitForVisibleElement(root, "RecentSessionsList", TimeSpan.FromSeconds(45));
		AutomationElement repeatAction = WaitForVisibleElement(
			root,
			Required("PFREMOTE_GOLDEN_REPEAT_SESSION_AUTOMATION_ID"),
			TimeSpan.FromSeconds(15));

        WriteResult(outputPath, new
        {
            schema_version = "pfremote.handoff-ui-automation/v1",
            status = "passed",
            agent_button_name = agentName,
            session_computer_name = computerName,
            session_action_name = actionName,
            recent_sessions_visible = !recentSessions.Current.IsOffscreen,
			repeat_action_visible = !repeatAction.Current.IsOffscreen,
            input_injection_used = false,
        });
        return 0;
    }

	private static void OpenSessions(AutomationElement root)
	{
		var sessionsCondition = new PropertyCondition(AutomationElement.AutomationIdProperty, "SessionsNavigationItem");
        AutomationElement? sessions = root.FindFirst(TreeScope.Descendants, sessionsCondition);
        if (sessions is null)
        {
            AutomationElement navigationToggle = WaitForElement(root, "TogglePaneButton", TimeSpan.FromSeconds(10));
            Invoke(navigationToggle);
            sessions = WaitForElement(root, "SessionsNavigationItem", TimeSpan.FromSeconds(15));
        }
        SelectOrInvoke(sessions);
	}

	private static void OpenSettings(AutomationElement root)
	{
		var condition = new PropertyCondition(AutomationElement.AutomationIdProperty, "SettingsNavigationItem");
		AutomationElement? settings = root.FindFirst(TreeScope.Descendants, condition);
		if (settings is null)
		{
			AutomationElement navigationToggle = WaitForElement(root, "TogglePaneButton", TimeSpan.FromSeconds(10));
			Invoke(navigationToggle);
			settings = WaitForElement(root, "SettingsNavigationItem", TimeSpan.FromSeconds(15));
		}
		SelectOrInvoke(settings);
	}

    private static AutomationElement WaitForWindow(int processId, TimeSpan timeout)
    {
        var condition = new PropertyCondition(AutomationElement.ProcessIdProperty, processId);
        DateTimeOffset deadline = DateTimeOffset.UtcNow + timeout;
        do
        {
            AutomationElementCollection windows = AutomationElement.RootElement.FindAll(TreeScope.Children, condition);
            foreach (AutomationElement window in windows.Cast<AutomationElement>()
                         .OrderByDescending(candidate => candidate.Current.ControlType == ControlType.Window)
                         .ThenByDescending(candidate => candidate.Current.BoundingRectangle.Width * candidate.Current.BoundingRectangle.Height))
            {
                if (window.Current.NativeWindowHandle != 0 &&
                    !window.Current.IsOffscreen &&
                    window.Current.BoundingRectangle.Width > 0 &&
                    window.Current.BoundingRectangle.Height > 0)
                {
                    return window;
                }
            }
            Thread.Sleep(100);
        }
        while (DateTimeOffset.UtcNow < deadline);
        throw new TimeoutException("PF Remote Center window was not visible on the isolated desktop.");
    }

    private static AutomationElement WaitForElement(AutomationElement root, string automationId, TimeSpan timeout)
    {
        var condition = new PropertyCondition(AutomationElement.AutomationIdProperty, automationId);
        DateTimeOffset deadline = DateTimeOffset.UtcNow + timeout;
        do
        {
            AutomationElement? element = root.FindFirst(TreeScope.Descendants, condition);
            if (element is not null && element.Current.IsEnabled)
            {
                return element;
            }
            Thread.Sleep(100);
        }
        while (DateTimeOffset.UtcNow < deadline);
        throw new TimeoutException($"PF Remote UI element was not ready: {automationId}. Root: {Describe(root)}. Buttons: {VisibleButtons(root)}. Windows: {VisibleWindows()}");
    }

    private static AutomationElement WaitForDesktopAction(AutomationElement root, TimeSpan timeout)
    {
        string automationId = Required("PFREMOTE_GOLDEN_DESKTOP_AUTOMATION_ID");
        string? deviceAlias = Environment.GetEnvironmentVariable("PFREMOTE_GOLDEN_DESKTOP_DEVICE_ALIAS");
        DateTimeOffset deadline = DateTimeOffset.UtcNow + timeout;
        do
        {
            AutomationElement? byId = root.FindFirst(
                TreeScope.Descendants,
                new PropertyCondition(AutomationElement.AutomationIdProperty, automationId));
            if (byId is not null && byId.Current.IsEnabled)
            {
                return byId;
            }
            if (!string.IsNullOrWhiteSpace(deviceAlias))
            {
                AutomationElement? byName = root.FindAll(TreeScope.Descendants, Condition.TrueCondition)
                    .Cast<AutomationElement>()
                    .FirstOrDefault(candidate =>
                        candidate.Current.IsEnabled &&
                        candidate.Current.IsKeyboardFocusable &&
                        candidate.Current.Name.Contains(deviceAlias, StringComparison.OrdinalIgnoreCase) &&
                        candidate.TryGetCurrentPattern(InvokePattern.Pattern, out _));
                if (byName is not null)
                {
                    return byName;
                }
            }
            Thread.Sleep(100);
        }
        while (DateTimeOffset.UtcNow < deadline);
        throw new TimeoutException($"PF Remote desktop action was not ready: {automationId}. Root: {Describe(root)}. Buttons: {VisibleButtons(root)}. Windows: {VisibleWindows()}");
    }

    private static AutomationElement WaitForVisibleElement(AutomationElement root, string automationId, TimeSpan timeout)
    {
        var condition = new PropertyCondition(AutomationElement.AutomationIdProperty, automationId);
        DateTimeOffset deadline = DateTimeOffset.UtcNow + timeout;
        do
        {
            AutomationElement? element = root.FindFirst(TreeScope.Descendants, condition);
            if (element is not null && element.Current.IsEnabled && !element.Current.IsOffscreen)
            {
                return element;
            }
            Thread.Sleep(100);
        }
        while (DateTimeOffset.UtcNow < deadline);
        throw new TimeoutException($"PF Remote UI element was not visible: {automationId}.");
    }

    private static void Invoke(AutomationElement element)
    {
        if (!element.TryGetCurrentPattern(InvokePattern.Pattern, out object pattern))
        {
            throw new InvalidOperationException($"PF Remote control cannot be invoked: {element.Current.AutomationId}");
        }
        ((InvokePattern)pattern).Invoke();
    }

    private static void SelectOrInvoke(AutomationElement element)
    {
        if (element.TryGetCurrentPattern(SelectionItemPattern.Pattern, out object selectionPattern))
        {
            ((SelectionItemPattern)selectionPattern).Select();
            return;
        }
        Invoke(element);
    }

    private static AutomationElement WaitForNamedButton(string name, TimeSpan timeout)
    {
        var condition = new AndCondition(
            new PropertyCondition(AutomationElement.ControlTypeProperty, ControlType.Button),
            new PropertyCondition(AutomationElement.NameProperty, name));
        DateTimeOffset deadline = DateTimeOffset.UtcNow + timeout;
        do
        {
            AutomationElement? button = AutomationElement.RootElement.FindFirst(TreeScope.Descendants, condition);
            if (button is not null && button.Current.IsEnabled)
            {
                return button;
            }
            Thread.Sleep(100);
        }
        while (DateTimeOffset.UtcNow < deadline);
        throw new TimeoutException($"PF Remote confirmation button was not ready: {name}.");
    }

    private static AutomationElement WaitForNamedElement(string name, ControlType controlType, TimeSpan timeout)
    {
        var condition = new AndCondition(
            new PropertyCondition(AutomationElement.ControlTypeProperty, controlType),
            new PropertyCondition(AutomationElement.NameProperty, name));
        DateTimeOffset deadline = DateTimeOffset.UtcNow + timeout;
        do
        {
            AutomationElement? element = AutomationElement.RootElement.FindFirst(TreeScope.Descendants, condition);
            if (element is not null && element.Current.IsEnabled)
            {
                return element;
            }
            Thread.Sleep(100);
        }
        while (DateTimeOffset.UtcNow < deadline);
        throw new TimeoutException($"PF Remote UI element was not ready: {name}.");
    }

    private static AutomationElement WaitForNamedElementAnyState(string name, ControlType controlType, TimeSpan timeout)
    {
        var condition = new AndCondition(
            new PropertyCondition(AutomationElement.ControlTypeProperty, controlType),
            new PropertyCondition(AutomationElement.NameProperty, name));
        DateTimeOffset deadline = DateTimeOffset.UtcNow + timeout;
        do
        {
            AutomationElement? element = AutomationElement.RootElement.FindFirst(TreeScope.Descendants, condition);
            if (element is not null && !element.Current.IsOffscreen)
            {
                return element;
            }
            Thread.Sleep(100);
        }
        while (DateTimeOffset.UtcNow < deadline);
        throw new TimeoutException($"PF Remote UI element was not visible: {name}.");
    }

    private static AutomationElement WaitForNamedAny(string name, TimeSpan timeout)
    {
        var condition = new PropertyCondition(AutomationElement.NameProperty, name);
        DateTimeOffset deadline = DateTimeOffset.UtcNow + timeout;
        do
        {
            AutomationElement? element = AutomationElement.RootElement.FindFirst(TreeScope.Descendants, condition);
            if (element is not null && element.Current.IsEnabled)
            {
                return element;
            }
            Thread.Sleep(100);
        }
        while (DateTimeOffset.UtcNow < deadline);
        throw new TimeoutException($"PF Remote UI element was not ready: {name}.");
    }

    private static string CurrentStatus(AutomationElement root)
    {
        var condition = new PropertyCondition(AutomationElement.AutomationIdProperty, "CatalogStatus");
        return root.FindFirst(TreeScope.Descendants, condition)?.Current.Name ?? "";
    }

	private static string WaitForStatus(AutomationElement root, string expected, TimeSpan timeout)
	{
		DateTimeOffset deadline = DateTimeOffset.UtcNow + timeout;
		do
		{
			string current = CurrentStatus(root);
			if (string.Equals(current, expected, StringComparison.Ordinal))
			{
				return current;
			}
			Thread.Sleep(50);
		}
		while (DateTimeOffset.UtcNow < deadline);
		throw new TimeoutException($"PF Remote status did not become '{expected}'. Current: '{CurrentStatus(root)}'.");
	}

	private static string WaitForStatusPrefix(AutomationElement root, string prefix, TimeSpan timeout)
	{
		DateTimeOffset deadline = DateTimeOffset.UtcNow + timeout;
		do
		{
			string current = CurrentStatus(root);
			if (current.StartsWith(prefix, StringComparison.Ordinal))
			{
				return current;
			}
			Thread.Sleep(50);
		}
		while (DateTimeOffset.UtcNow < deadline);
		throw new TimeoutException($"PF Remote status did not start with '{prefix}'. Current: '{CurrentStatus(root)}'.");
	}

    private static string VisibleButtons(AutomationElement root)
    {
        var condition = new PropertyCondition(AutomationElement.ControlTypeProperty, ControlType.Button);
        AutomationElementCollection buttons = root.FindAll(TreeScope.Descendants, condition);
        return string.Join("; ", buttons.Cast<AutomationElement>().Take(40).Select(button =>
            $"id={button.Current.AutomationId},name={button.Current.Name},enabled={button.Current.IsEnabled}"));
    }

    private static string VisibleWindows()
    {
        AutomationElementCollection windows = AutomationElement.RootElement.FindAll(TreeScope.Children, Condition.TrueCondition);
        return string.Join("; ", windows.Cast<AutomationElement>().Take(20).Select(Describe));
    }

	private static (string ProcessName, string WindowName) WaitForDesktopExecutorWindow(
		HashSet<int> existingProcesses,
		TimeSpan timeout)
	{
		DateTimeOffset deadline = DateTimeOffset.UtcNow + timeout;
		do
		{
			AutomationElementCollection windows = AutomationElement.RootElement.FindAll(TreeScope.Children, Condition.TrueCondition);
			foreach (AutomationElement window in windows.Cast<AutomationElement>())
			{
				int processId = window.Current.ProcessId;
				if (processId <= 0 || existingProcesses.Contains(processId) || window.Current.IsOffscreen)
				{
					continue;
				}
				try
				{
					string processName = System.Diagnostics.Process.GetProcessById(processId).ProcessName;
					if (string.Equals(processName, "vncviewer", StringComparison.OrdinalIgnoreCase) ||
						string.Equals(processName, "mstsc", StringComparison.OrdinalIgnoreCase))
					{
						return (processName, window.Current.Name);
					}
				}
				catch (ArgumentException)
				{
					// The candidate exited between UI Automation discovery and process inspection.
				}
			}
			Thread.Sleep(50);
		}
		while (DateTimeOffset.UtcNow < deadline);
		throw new TimeoutException($"PF Remote Desktop executor did not show a window. Windows: {VisibleWindows()}");
	}

    private static string Describe(AutomationElement element) =>
        $"pid={element.Current.ProcessId},id={element.Current.AutomationId},name={element.Current.Name}," +
        $"type={element.Current.ControlType.ProgrammaticName},offscreen={element.Current.IsOffscreen},handle={element.Current.NativeWindowHandle}," +
        $"bounds={element.Current.BoundingRectangle}";

    private static void WaitForFile(string path, TimeSpan timeout)
    {
        DateTimeOffset deadline = DateTimeOffset.UtcNow + timeout;
        do
        {
            if (File.Exists(path) && new FileInfo(path).Length > 0)
            {
                return;
            }
            Thread.Sleep(100);
        }
        while (DateTimeOffset.UtcNow < deadline);
        throw new TimeoutException($"PF Remote action evidence was not produced: {Path.GetFileName(path)}");
    }

    private static string Required(string name) =>
        Environment.GetEnvironmentVariable(name) is { Length: > 0 } value
            ? value
            : throw new InvalidOperationException($"Missing isolated golden-journey setting: {name}");

    private static void WriteResult(string path, object value)
    {
        string fullPath = Path.GetFullPath(path);
        Directory.CreateDirectory(Path.GetDirectoryName(fullPath) ?? throw new InvalidOperationException("Golden UI result path has no directory."));
        string json = JsonSerializer.Serialize(value, JsonOptions) + Environment.NewLine;
        File.WriteAllText(fullPath, json, new UTF8Encoding(false));
    }
}
