using System.Diagnostics;
using System.Text.Json;

using PFRemoteCenter.Models;

namespace PFRemoteCenter.Services;

internal sealed class PfRemoteLifecycleClient
{
    private readonly string _helperPath;

    internal PfRemoteLifecycleClient()
        : this(Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
            "PFRemote",
            "Installer",
            "PFRemoteSetup.exe"))
    {
    }

    internal PfRemoteLifecycleClient(string helperPath)
    {
        _helperPath = helperPath;
    }

    internal bool IsAvailable => File.Exists(_helperPath);

    internal Task<InstalledVersionResponse> StatusAsync(CancellationToken cancellationToken = default) =>
        RunAsync("status", cancellationToken);

    internal Task<InstalledVersionResponse> RollbackAsync(CancellationToken cancellationToken = default) =>
        RunAsync("rollback", cancellationToken);

	internal Task<InstalledVersionResponse> StartupAsync(CancellationToken cancellationToken = default) =>
		RunAsync("startup", cancellationToken);

    private async Task<InstalledVersionResponse> RunAsync(string action, CancellationToken cancellationToken)
    {
        if (!IsAvailable)
        {
            throw new FileNotFoundException("PF Remote installation maintenance is unavailable.", _helperPath);
        }
		ProcessStartInfo startInfo = CreateStartInfo(_helperPath, action);
        using Process process = Process.Start(startInfo)
            ?? throw new InvalidOperationException("PF Remote installation maintenance did not start.");
        string output = await process.StandardOutput.ReadToEndAsync(cancellationToken).ConfigureAwait(false);
        string error = await process.StandardError.ReadToEndAsync(cancellationToken).ConfigureAwait(false);
        await process.WaitForExitAsync(cancellationToken).ConfigureAwait(false);
        if (process.ExitCode != 0)
        {
            throw new InvalidOperationException(string.IsNullOrWhiteSpace(error)
                ? "PF Remote installation maintenance did not complete."
                : error.Trim());
        }
        return JsonSerializer.Deserialize<InstalledVersionResponse>(output)
            ?? throw new InvalidOperationException("PF Remote installation maintenance returned an invalid result.");
    }

	internal static ProcessStartInfo CreateStartInfo(string helperPath, string action)
	{
		var startInfo = new ProcessStartInfo
		{
			FileName = helperPath,
			UseShellExecute = false,
			CreateNoWindow = true,
			RedirectStandardOutput = true,
			RedirectStandardError = true,
		};
		startInfo.ArgumentList.Add(action);
		startInfo.ArgumentList.Add("--no-launch");
		return startInfo;
	}
}
