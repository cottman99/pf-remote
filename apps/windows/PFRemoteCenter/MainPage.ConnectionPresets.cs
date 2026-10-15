using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Controls.Primitives;
using Microsoft.UI.Xaml.Automation;
using PFRemoteCenter.Models;
using PFRemoteCenter.Presentation;
using PFRemoteCenter.Services;
using System.Globalization;

namespace PFRemoteCenter;

public sealed partial class MainPage
{
    private void AddPresetMenuItem(MenuFlyout menu, string? target)
    {
        if (target is null || !_catalogTargets.Any(t => t.Canonical == target && t.Capability.DesktopProfile?.Protocol == "rdp")) return;
        menu.Items.Add(new MenuFlyoutSeparator());
        var item = new MenuFlyoutItem { Text = _resources.GetString("ConnectionPresetTitle") };
        item.Click += async (_, _) => await ShowConnectionPresetAsync(target);
        menu.Items.Add(item);
    }

    private async Task<bool> ShowConnectionPresetAsync(string? initialTarget, bool saveAndConnect = false)
    {
        var selectedDevice = _catalogTargets.FirstOrDefault(t => t.Canonical == initialTarget)?.Device.Id;
        var targets = _catalogTargets.Where(t => t.Capability.DesktopProfile?.Protocol == "rdp" &&
            (selectedDevice is null || t.Device.Id == selectedDevice)).ToArray();
        var chooser = new ComboBox { Header = _resources.GetString("PresetDesktopLabel"), HorizontalAlignment = HorizontalAlignment.Stretch };
        foreach (var target in targets)
            chooser.Items.Add(new ComboBoxItem { Content = target.Device.DisplayName + " · " + target.Capability.DisplayName, Tag = target.Canonical });
        var username = new TextBox { Header = _resources.GetString("PresetUsernameLabel"), MaxLength = 256 };
        var password = new PasswordBox { Header = _resources.GetString("DesktopPasswordLabel"), MaxLength = 1200 };
        var saved = new TextBlock { TextWrapping = TextWrapping.Wrap };
        var workspaceHelp = new TextBlock { Text = _resources.GetString("PresetWorkspaceHelp"), TextWrapping = TextWrapping.Wrap, Visibility = Visibility.Collapsed };
        var clear = new CheckBox { Content = _resources.GetString("PresetClearPassword") };
        var displayMode = new ComboBox { Header = _resources.GetString("PresetDisplayMode"), HorizontalAlignment = HorizontalAlignment.Stretch };
        foreach (var (mode, label) in new[] { ("window", "PresetDisplayWindow"), ("single", "PresetDisplaySingle"), ("all", "PresetDisplayAll"), ("selected", "PresetDisplaySelected") })
        {
            var option = new ComboBoxItem { Content = _resources.GetString(label), Tag = mode };
            AutomationProperties.SetAutomationId(option, "preset-display-" + mode);
            displayMode.Items.Add(option);
        }
        displayMode.SelectedIndex = 1;
        var screens = new StackPanel { Spacing = 4 };
        var screenMap = new Canvas { Width = 320, Height = 120, HorizontalAlignment = HorizontalAlignment.Left };
        var displayHelp = new TextBlock { Text = _resources.GetString("PresetDisplayHelp"), TextWrapping = TextWrapping.Wrap };
        var screenHelp = new TextBlock { TextWrapping = TextWrapping.Wrap };
        var refreshScreens = new Button { Content = _resources.GetString("PresetRefreshScreens") };
        var screenSelection = new StackPanel { Spacing = 8, Visibility = Visibility.Collapsed };
        foreach (var element in new UIElement[] { screenHelp, screenMap, screens, refreshScreens }) screenSelection.Children.Add(element);
        DesktopMonitorLayout? layout = null;
        var selectedScreens = new List<int>();
        var preferred = new CheckBox { Content = _resources.GetString("PresetDefaultDesktop") };
        var status = new InfoBar { IsClosable = false, IsOpen = false, Severity = InfoBarSeverity.Error };
        var trust = new Button { Content = _resources.GetString("PresetRepairTrust"), IsEnabled = _lifecycleClient.IsAvailable };
        trust.Click += async (_, _) => {
            trust.IsEnabled = false;
            try { await _lifecycleClient.PrepareDesktopTrustAsync(); status.Severity = InfoBarSeverity.Success; status.Message = _resources.GetString("PresetTrustReady"); }
            catch { status.Severity = InfoBarSeverity.Error; status.Message = _resources.GetString("PresetTrustFailed"); }
            finally { status.IsOpen = true; trust.IsEnabled = true; }
        };
        var form = new StackPanel { Spacing = 12, MinWidth = 260 };
        foreach (var element in new UIElement[] { chooser, workspaceHelp, username, password, saved, clear, displayMode, displayHelp, screenSelection, preferred,
            new TextBlock { Text = _resources.GetString("PresetSharingHelp"), TextWrapping = TextWrapping.Wrap }, trust, status }) form.Children.Add(element);
        AutomationProperties.SetAutomationId(username, "preset-username");
        AutomationProperties.SetAutomationId(password, "preset-password");
        AutomationProperties.SetAutomationId(chooser, "preset-desktop");
        AutomationProperties.SetAutomationId(displayMode, "preset-display-mode");
        AutomationProperties.SetAutomationId(refreshScreens, "preset-refresh-screens");
        var dialog = new ContentDialog {
            XamlRoot = XamlRoot, Title = _resources.GetString("ConnectionPresetTitle"),
            Content = new ScrollViewer { Content = form, MaxHeight = 560, HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled },
            PrimaryButtonText = _resources.GetString(saveAndConnect ? "PresetSaveAndConnect" : "PresetSave"), CloseButtonText = _resources.GetString("CancelButtonLabel"),
            DefaultButton = ContentDialogButton.Primary, IsPrimaryButtonEnabled = false
        };
        AutomationProperties.SetAutomationId(dialog, "connection-preset-dialog");
#if DEBUG
        if (Environment.GetEnvironmentVariable("PFREMOTE_VISUAL_AUDIT_STATE") == "connection-presets")
            dialog.Opened += async (_, _) => await VisualAuditCapture.CaptureWhenRequestedAsync(dialog);
#endif
        int revision = 0;
        bool passwordSaved = false;
        bool loaded = false;
        string CurrentMode() => (displayMode.SelectedItem as ComboBoxItem)?.Tag as string ?? "single";
        void UpdateDisplayState()
        {
            bool selected = CurrentMode() == "selected";
            screenSelection.Visibility = selected ? Visibility.Visible : Visibility.Collapsed;
            string? error = DisplayPresetPresenter.SelectionError(CurrentMode(), selectedScreens, layout);
            screenHelp.Text = _resources.GetString(error ?? "PresetSelectedScreensHelp");
            if (selected && error is null && selectedScreens.Count > 0)
                screenHelp.Text += " " + string.Format(CultureInfo.CurrentCulture, _resources.GetString("PresetRemotePrimaryScreen"), selectedScreens[0] + 1);
            dialog.IsPrimaryButtonEnabled = loaded && error is null;
        }
        void RenderScreens(DesktopMonitorLayout? current, IReadOnlyList<int> selection)
        {
            layout = current;
            selectedScreens.Clear(); selectedScreens.AddRange(selection);
            screens.Children.Clear();
            screenMap.Children.Clear();
            screenMap.Visibility = current?.Monitors.Count > 0 ? Visibility.Visible : Visibility.Collapsed;
            if (current is not null)
                foreach (var monitor in current.Monitors)
                {
                    string label = string.Format(CultureInfo.CurrentCulture, _resources.GetString("PresetScreenLabel"), monitor.Id + 1, monitor.Right - monitor.Left, monitor.Bottom - monitor.Top);
                    if (monitor.Left == 0 && monitor.Top == 0) label += _resources.GetString("PresetPrimaryScreen");
                    var check = new CheckBox { Content = new TextBlock { Text = label, TextWrapping = TextWrapping.Wrap }, IsChecked = selectedScreens.Contains(monitor.Id) };
                    AutomationProperties.SetAutomationId(check, "preset-screen-" + monitor.Id);
                    AutomationProperties.SetName(check, label);
                    check.Checked += (_, _) => { if (!selectedScreens.Contains(monitor.Id)) selectedScreens.Add(monitor.Id); UpdateDisplayState(); };
                    check.Unchecked += (_, _) => { selectedScreens.Remove(monitor.Id); UpdateDisplayState(); };
                    screens.Children.Add(check);
                    int left = current.Monitors.Min(m => m.Left), top = current.Monitors.Min(m => m.Top);
                    double scale = Math.Min(320.0 / (current.Monitors.Max(m => m.Right) - left), 120.0 / (current.Monitors.Max(m => m.Bottom) - top));
                    var tile = new ToggleButton { Content = string.Format(CultureInfo.CurrentCulture, _resources.GetString("PresetScreenNumber"), monitor.Id + 1), IsChecked = check.IsChecked,
                        Width = Math.Max(1, (monitor.Right - monitor.Left) * scale - 4), Height = Math.Max(1, (monitor.Bottom - monitor.Top) * scale - 4), Padding = new Thickness(2) };
                    AutomationProperties.SetName(tile, label);
                    tile.Checked += (_, _) => check.IsChecked = true;
                    tile.Unchecked += (_, _) => check.IsChecked = false;
                    check.Checked += (_, _) => tile.IsChecked = true;
                    check.Unchecked += (_, _) => tile.IsChecked = false;
                    Canvas.SetLeft(tile, (monitor.Left - left) * scale); Canvas.SetTop(tile, (monitor.Top - top) * scale);
                    screenMap.Children.Add(tile);
                }
            UpdateDisplayState();
        }
        displayMode.SelectionChanged += (_, _) => UpdateDisplayState();
        refreshScreens.Click += async (_, _) => {
            if (chooser.SelectedItem is not ComboBoxItem { Tag: string target }) return;
            int current = revision;
            refreshScreens.IsEnabled = false;
            try {
                var response = await _client.ReadPresetAsync(target);
                if (current != revision) return;
                RenderScreens(response.Displays, layout?.Id == response.Displays?.Id ? selectedScreens.ToArray() : []);
            } catch { if (current == revision) { RenderScreens(null, []); } }
            finally { if (current == revision) refreshScreens.IsEnabled = loaded; }
        };
        chooser.SelectionChanged += async (_, _) => {
            int current = ++revision;
            loaded = false; dialog.IsPrimaryButtonEnabled = false; status.IsOpen = false;
            displayMode.IsEnabled = false; refreshScreens.IsEnabled = false;
            RenderScreens(null, []);
            password.Password = string.Empty; username.Text = string.Empty; clear.IsChecked = false;
            saved.Text = string.Empty;
            passwordSaved = false;
            if (chooser.SelectedItem is not ComboBoxItem { Tag: string target }) return;
            bool vncWorkspace = targets.First(t => t.Canonical == target).Capability.DesktopProfile?.SessionSelector?.StartsWith("pfremote-shared-", StringComparison.Ordinal) == true;
            workspaceHelp.Visibility = vncWorkspace ? Visibility.Visible : Visibility.Collapsed;
            username.Visibility = vncWorkspace ? Visibility.Collapsed : Visibility.Visible;
            password.Header = _resources.GetString(vncWorkspace ? "PresetWorkspacePassword" : "DesktopPasswordLabel");
            try {
                var preset = await _client.ReadPresetAsync(target);
                if (current != revision) return;
                username.Text = vncWorkspace ? "na" : preset.Username; preferred.IsChecked = preset.DefaultDesktop;
                string mode = DisplayPresetPresenter.Mode(preset);
                displayMode.SelectedItem = displayMode.Items.Cast<ComboBoxItem>().FirstOrDefault(item => item.Tag as string == mode);
                loaded = true;
                RenderScreens(preset.Displays, DisplayPresetPresenter.RestoreSelection(preset.Display, preset.Displays));
                if (mode == "selected" && preset.Display?.LayoutId != preset.Displays?.Id)
                    screenHelp.Text = _resources.GetString("PresetDisplaysChanged");
                passwordSaved = preset.PasswordSaved && !preset.RequiresSetup && username.Text == preset.Username;
                saved.Text = _resources.GetString(preset.RequiresSetup ? "PresetIdentityChanged" : passwordSaved ? "PresetPasswordSaved" : vncWorkspace ? "PresetWorkspacePasswordMissing" : "PresetPasswordMissing");
            } catch { if (current == revision) { status.Message = _resources.GetString("PresetLoadFailed"); status.IsOpen = true; } }
            finally { if (current == revision) { displayMode.IsEnabled = loaded; refreshScreens.IsEnabled = loaded; } }
        };
        dialog.PrimaryButtonClick += async (_, args) => {
            var deferral = args.GetDeferral();
            chooser.IsEnabled = false;
            try {
                if (chooser.SelectedItem is not ComboBoxItem { Tag: string target }) { args.Cancel = true; return; }
                bool workspace = targets.First(t => t.Canonical == target).Capability.DesktopProfile?.SessionSelector?.Length > 0;
                if (saveAndConnect && workspace && (clear.IsChecked == true || (!passwordSaved && password.Password.Length == 0))) {
                    args.Cancel = true; status.Severity = InfoBarSeverity.Error;
                    status.Message = _resources.GetString("PresetWorkspacePasswordMissing"); status.IsOpen = true; return;
                }
                string mode = CurrentMode();
                string? displayError = DisplayPresetPresenter.SelectionError(mode, selectedScreens, layout);
                if (displayError is not null) { args.Cancel = true; screenHelp.Text = _resources.GetString(displayError); return; }
                await _client.SavePresetAsync(target, new DesktopPresetUpdate(username.Text, password.Password,
                    clear.IsChecked == true, mode != "window", preferred.IsChecked == true,
                    DisplayPresetPresenter.Build(mode, selectedScreens, layout)));
                password.Password = string.Empty;
            } catch (PfRemoteCliException exception) when (exception.Code == "DESKTOP_DISPLAY_CHANGED") {
                args.Cancel = true; status.Severity = InfoBarSeverity.Warning; status.Message = _resources.GetString("PresetDisplaysChanged"); status.IsOpen = true;
            } catch { args.Cancel = true; status.Severity = InfoBarSeverity.Error; status.Message = _resources.GetString("PresetSaveFailed"); status.IsOpen = true; }
            finally { chooser.IsEnabled = !saveAndConnect; deferral.Complete(); }
        };
        int index = Array.FindIndex(targets, t => t.Canonical == initialTarget);
        chooser.SelectedIndex = targets.Length == 0 ? -1 : Math.Max(0, index);
        chooser.IsEnabled = !saveAndConnect;
        if (targets.Length == 0) { status.Message = _resources.GetString("PresetNoDesktop"); status.IsOpen = true; }
        var result = await ShowDialogAsync(dialog);
        revision++;
        password.Password = string.Empty;
        await RefreshCatalogAsync();
        return result == ContentDialogResult.Primary;
    }
}
