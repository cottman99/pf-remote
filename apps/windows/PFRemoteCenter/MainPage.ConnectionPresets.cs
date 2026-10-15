using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Automation;
using PFRemoteCenter.Models;
using PFRemoteCenter.Presentation;

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
        var fullScreen = new CheckBox { Content = _resources.GetString("PresetFullScreen"), IsChecked = true };
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
        foreach (var element in new UIElement[] { chooser, workspaceHelp, username, password, saved, clear, fullScreen, preferred,
            new TextBlock { Text = _resources.GetString("PresetSharingHelp"), TextWrapping = TextWrapping.Wrap }, trust, status }) form.Children.Add(element);
        AutomationProperties.SetAutomationId(username, "preset-username");
        AutomationProperties.SetAutomationId(password, "preset-password");
        var dialog = new ContentDialog {
            XamlRoot = XamlRoot, Title = _resources.GetString("ConnectionPresetTitle"),
            Content = new ScrollViewer { Content = form, MaxHeight = 560, HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled },
            PrimaryButtonText = _resources.GetString(saveAndConnect ? "PresetSaveAndConnect" : "PresetSave"), CloseButtonText = _resources.GetString("CancelButtonLabel"),
            DefaultButton = ContentDialogButton.Primary, IsPrimaryButtonEnabled = false
        };
#if DEBUG
        if (Environment.GetEnvironmentVariable("PFREMOTE_VISUAL_AUDIT_STATE") == "connection-presets")
            dialog.Opened += async (_, _) => await VisualAuditCapture.CaptureWhenRequestedAsync(dialog);
#endif
        int revision = 0;
        bool passwordSaved = false;
        chooser.SelectionChanged += async (_, _) => {
            int current = ++revision;
            dialog.IsPrimaryButtonEnabled = false; status.IsOpen = false;
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
                username.Text = vncWorkspace ? "na" : preset.Username; fullScreen.IsChecked = preset.FullScreen; preferred.IsChecked = preset.DefaultDesktop;
                passwordSaved = preset.PasswordSaved && !preset.RequiresSetup && username.Text == preset.Username;
                saved.Text = _resources.GetString(preset.RequiresSetup ? "PresetIdentityChanged" : passwordSaved ? "PresetPasswordSaved" : vncWorkspace ? "PresetWorkspacePasswordMissing" : "PresetPasswordMissing");
                dialog.IsPrimaryButtonEnabled = true;
            } catch { if (current == revision) { status.Message = _resources.GetString("PresetLoadFailed"); status.IsOpen = true; } }
        };
        dialog.PrimaryButtonClick += async (_, args) => {
            var deferral = args.GetDeferral();
            chooser.IsEnabled = false;
            try {
                if (chooser.SelectedItem is not ComboBoxItem { Tag: string target }) { args.Cancel = true; return; }
                if (saveAndConnect && (clear.IsChecked == true || (!passwordSaved && password.Password.Length == 0))) {
                    args.Cancel = true; status.Severity = InfoBarSeverity.Error;
                    status.Message = _resources.GetString("PresetWorkspacePasswordMissing"); status.IsOpen = true; return;
                }
                await _client.SavePresetAsync(target, new DesktopPresetUpdate(username.Text, password.Password,
                    clear.IsChecked == true, fullScreen.IsChecked == true, preferred.IsChecked == true));
                password.Password = string.Empty;
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
