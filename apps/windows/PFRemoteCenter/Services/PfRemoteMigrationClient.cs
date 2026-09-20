using System.Diagnostics;
using System.Text.Json;

using PFRemoteCenter.Models;

namespace PFRemoteCenter.Services;

internal sealed class PfRemoteMigrationClient
{
    private static readonly JsonSerializerOptions JsonOptions = new(JsonSerializerDefaults.Web);
    private readonly string _executablePath = Path.Combine(AppContext.BaseDirectory, "pfremote-migrate.exe");

    public static string LegacyCenterCatalogPath
    {
        get
        {
            string? selected = Environment.GetEnvironmentVariable("PFREMOTE_LEGACY_CENTER_CATALOG");
            return string.IsNullOrWhiteSpace(selected)
                ? Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData), "PFRemoteCenter", "catalog.json")
                : Path.GetFullPath(selected);
        }
    }

    public async Task<LegacyInventoryResponse?> PreviewExistingSetupAsync(CancellationToken cancellationToken = default)
    {
        string path = LegacyCenterCatalogPath;
        if (!File.Exists(path))
        {
            return null;
        }
        if (!File.Exists(_executablePath))
        {
            throw new FileNotFoundException("PF Remote migration helper is not present.", _executablePath);
        }

        return await RunAsync<LegacyInventoryResponse>(CreateExportArguments(path), cancellationToken);
    }

    public async Task<bool> IsExistingSetupEnabledAsync(CancellationToken cancellationToken = default)
    {
        LegacyCompatibilityStatus status = await RunAsync<LegacyCompatibilityStatus>(CreateStatusArguments(), cancellationToken);
        return status.Enabled;
    }

    public async Task EnableExistingSetupAsync(CancellationToken cancellationToken = default)
    {
        _ = await RunAsync<JsonElement>(CreateEnableArguments(), cancellationToken);
    }

    public async Task DisableExistingSetupAsync(CancellationToken cancellationToken = default)
    {
        _ = await RunAsync<JsonElement>(CreateDisableArguments(), cancellationToken);
    }

    public async Task ConfigureConnectionServiceAsync(string invitationPath, CancellationToken cancellationToken = default)
    {
        _ = await RunAsync<JsonElement>(CreateConfigureConnectionServiceArguments(invitationPath), cancellationToken);
    }

    private async Task<T> RunAsync<T>(IReadOnlyList<string> arguments, CancellationToken cancellationToken)
    {
        if (!File.Exists(_executablePath))
        {
            throw new FileNotFoundException("PF Remote migration helper is not present.", _executablePath);
        }
        ProcessStartInfo startInfo = PfRemoteCliClient.CreateStartInfo(_executablePath, arguments);
        using var process = Process.Start(startInfo) ?? throw new InvalidOperationException("PF Remote migration preview did not start.");
        string output = await process.StandardOutput.ReadToEndAsync(cancellationToken);
        string error = await process.StandardError.ReadToEndAsync(cancellationToken);
        await process.WaitForExitAsync(cancellationToken);
        if (process.ExitCode != 0)
        {
            throw new InvalidOperationException(string.IsNullOrWhiteSpace(error) ? "PF Remote could not safely preview the existing setup." : error.Trim());
        }
        return JsonSerializer.Deserialize<T>(output, JsonOptions)
            ?? throw new InvalidDataException("PF Remote migration preview returned an empty response.");
    }

    internal static IReadOnlyList<string> CreateExportArguments(string catalogPath) =>
        ["export-legacy-center", "--input", catalogPath];

    internal static IReadOnlyList<string> CreateStatusArguments() => ["status-legacy-center"];

    internal static IReadOnlyList<string> CreateEnableArguments() => ["enable-legacy-center"];

    internal static IReadOnlyList<string> CreateDisableArguments() => ["disable-legacy-center"];

    internal static IReadOnlyList<string> CreateConfigureConnectionServiceArguments(string invitationPath) =>
        ["configure-connection-service", "--invitation", invitationPath];
}
