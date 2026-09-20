using System.Diagnostics;
using System.Diagnostics.CodeAnalysis;

namespace PFRemoteCenter.Services;

[SuppressMessage("Design", "CA1001:Types that own disposable fields should be disposable", Justification = "The daemon coordinator and its async gate live for the process lifetime.")]
internal sealed class PfRemoteDaemon
{
    private static readonly TimeSpan StartupTimeout = TimeSpan.FromSeconds(5);
    private static readonly TimeSpan PollInterval = TimeSpan.FromMilliseconds(100);

    private readonly string _daemonPath;
    private readonly PfRemoteCliClient _client;
	private readonly SemaphoreSlim _startGate = new(1, 1);

    internal PfRemoteDaemon()
        : this(Path.Combine(AppContext.BaseDirectory, "pfremoted.exe"), new PfRemoteCliClient())
    {
    }

    internal PfRemoteDaemon(string daemonPath, PfRemoteCliClient client)
    {
        _daemonPath = daemonPath;
        _client = client;
    }

    internal async Task EnsureStartedAsync(CancellationToken cancellationToken = default)
    {
        if (await IsReadyAsync(cancellationToken))
        {
            return;
        }

		await _startGate.WaitAsync(cancellationToken).ConfigureAwait(false);
		try
		{
			if (await IsReadyAsync(cancellationToken).ConfigureAwait(false))
			{
				return;
			}
			if (!File.Exists(_daemonPath))
			{
				throw new FileNotFoundException("PF Remote background service is not present beside Center.", _daemonPath);
			}

			_ = Process.Start(CreateStartInfo(_daemonPath))
				?? throw new InvalidOperationException("PF Remote background service did not start.");

			using var startup = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
			startup.CancelAfter(StartupTimeout);
			while (!startup.IsCancellationRequested)
			{
				try
				{
					await Task.Delay(PollInterval, startup.Token).ConfigureAwait(false);
				}
				catch (OperationCanceledException) when (!cancellationToken.IsCancellationRequested)
				{
					break;
				}
				if (await IsReadyAsync(startup.Token).ConfigureAwait(false))
				{
					return;
				}
			}
			cancellationToken.ThrowIfCancellationRequested();
			throw new TimeoutException("PF Remote background service did not become ready.");
		}
		finally
		{
			_startGate.Release();
		}
    }

    private async Task<bool> IsReadyAsync(CancellationToken cancellationToken)
    {
        try
        {
            _ = await _client.ListAsync(cancellationToken).ConfigureAwait(false);
            return true;
        }
        catch (Exception) when (!cancellationToken.IsCancellationRequested)
        {
            return false;
        }
    }

    internal static ProcessStartInfo CreateStartInfo(string daemonPath) => new()
    {
        FileName = daemonPath,
        WorkingDirectory = Path.GetDirectoryName(daemonPath) ?? AppContext.BaseDirectory,
        CreateNoWindow = true,
        UseShellExecute = false,
    };
}
