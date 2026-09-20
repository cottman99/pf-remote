using System.Diagnostics.CodeAnalysis;

using Microsoft.UI.Xaml;
using Microsoft.UI.Windowing;
using System.Diagnostics;
using System.Security.Principal;
using PFRemoteCenter.Services;
#if DEBUG
using Windows.Graphics;
#endif

namespace PFRemoteCenter;

[SuppressMessage("Design", "CA1001:Types that own disposable fields should be disposable", Justification = "The background-start gate and tray icon live for the application process lifetime.")]
public partial class App : Application
{
    private Window? _window;
    private SingleInstanceService? _singleInstance;
	private TrayIconService? _trayIcon;
	private readonly PfRemoteDaemon _daemon = new();
	private readonly PfRemoteLifecycleClient _lifecycle = new();
	private readonly SemaphoreSlim _backgroundStartGate = new(1, 1);

	internal Task DaemonReady { get; private set; } = Task.CompletedTask;

	internal async Task EnsureBackgroundStartedAsync(CancellationToken cancellationToken = default)
	{
		await _backgroundStartGate.WaitAsync(cancellationToken);
		try
		{
			if (_lifecycle.IsAvailable)
			{
				try
				{
					_ = await _lifecycle.StartupAsync(cancellationToken);
				}
				catch (Exception) when (!cancellationToken.IsCancellationRequested)
				{
					// Development builds and partially installed copies can still
					// start their adjacent daemon even when setup maintenance is absent.
				}
			}
			await _daemon.EnsureStartedAsync(cancellationToken);
		}
		finally
		{
			_backgroundStartGate.Release();
		}
	}

	internal Window? ActiveWindow => _window;

    public App()
    {
        InitializeComponent();
    }

    protected override void OnLaunched(LaunchActivatedEventArgs args)
    {
        using WindowsIdentity identity = WindowsIdentity.GetCurrent();
        using Process process = Process.GetCurrentProcess();
        string scope = $"{identity.User?.Value ?? throw new InvalidOperationException("Missing Windows identity.")}|{process.SessionId}";
#if DEBUG
        // Synthetic protected-IPC fixtures must not activate the installed client.
        if (Environment.GetEnvironmentVariable("PFREMOTE_LOCAL_ENDPOINT") is { Length: > 0 } endpoint)
            scope += $"|{endpoint}";
#endif
        _singleInstance = new SingleInstanceService(scope);
        bool backgroundStart = !SingleInstanceService.ShouldShow(Environment.GetCommandLineArgs().Skip(1));
        if (!_singleInstance.IsPrimary)
        {
            if (!backgroundStart) _singleInstance.RequestActivation();
            _singleInstance.Dispose();
            _singleInstance = null;
            Exit();
            return;
        }
		DaemonReady = EnsureBackgroundStartedAsync();
        _window = new MainWindow();
        _singleInstance.Listen(() => _window.DispatcherQueue.TryEnqueue(ShowExistingWindow));
#if DEBUG
		if (!string.Equals(Environment.GetEnvironmentVariable("PFREMOTE_VISUAL_AUDIT_NO_TRAY"), "true", StringComparison.OrdinalIgnoreCase))
		{
			_trayIcon = new TrayIconService(_window);
		}
#else
        _trayIcon = new TrayIconService(_window);
#endif
		if (!backgroundStart)
		{
			_window.Activate();
		}
#if DEBUG
		if (int.TryParse(Environment.GetEnvironmentVariable("PFREMOTE_VISUAL_AUDIT_WIDTH"), out int width) &&
			int.TryParse(Environment.GetEnvironmentVariable("PFREMOTE_VISUAL_AUDIT_HEIGHT"), out int height) &&
			width >= 480 && height >= 480)
		{
			_window.AppWindow.Resize(new SizeInt32(width, height));
		}
#endif
    }

    private void ShowExistingWindow()
    {
        if (_window is null) return;
        if (_window.AppWindow.Presenter is OverlappedPresenter presenter &&
            presenter.State == OverlappedPresenterState.Minimized)
            presenter.Restore();
        _window.AppWindow.Show();
        _window.Activate();
    }
}
