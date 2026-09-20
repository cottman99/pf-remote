using System.Diagnostics;

namespace PFRemoteCenter.Services;

internal sealed class PfRemoteRecoveryClient
{
    private readonly string _executablePath = Path.Combine(AppContext.BaseDirectory, "pfremote-recovery.exe");

    public Task<byte[]> ExportAsync(string password, CancellationToken cancellationToken = default) =>
        RunAsync(["export"], password, cancellationToken);

    public async Task RestoreAsync(string recoveryFile, string password, CancellationToken cancellationToken = default) =>
        _ = await RunAsync(CreateRestoreArguments(recoveryFile), password, cancellationToken);

    private async Task<byte[]> RunAsync(IReadOnlyList<string> arguments, string password, CancellationToken cancellationToken)
    {
        if (!File.Exists(_executablePath))
        {
            throw new FileNotFoundException("PF Remote recovery helper is not present.", _executablePath);
        }

        ProcessStartInfo startInfo = PfRemoteCliClient.CreateStartInfo(_executablePath, arguments);
        startInfo.RedirectStandardInput = true;
        using var process = Process.Start(startInfo) ?? throw new InvalidOperationException("PF Remote recovery did not start.");
        await process.StandardInput.WriteLineAsync(password.AsMemory(), cancellationToken);
        process.StandardInput.Close();
        using var output = new MemoryStream();
        Task copyOutput = process.StandardOutput.BaseStream.CopyToAsync(output, cancellationToken);
        string error = await process.StandardError.ReadToEndAsync(cancellationToken);
        await Task.WhenAll(copyOutput, process.WaitForExitAsync(cancellationToken));
        if (process.ExitCode != 0)
        {
            throw new InvalidOperationException(string.IsNullOrWhiteSpace(error) ? "PF Remote recovery failed." : error.Trim());
        }
        return output.ToArray();
    }

    internal static IReadOnlyList<string> CreateRestoreArguments(string recoveryFile) =>
        ["restore", "--file", recoveryFile];
}
