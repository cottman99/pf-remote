using System.Diagnostics;
using System.Text;
using System.Text.Json;

using PFRemoteCenter.Models;

namespace PFRemoteCenter.Services;

internal sealed class PfRemoteCliClient
{
    private static readonly JsonSerializerOptions JsonOptions = new(JsonSerializerDefaults.Web);

    private readonly string _executablePath = Path.Combine(AppContext.BaseDirectory, "pfremote.exe");

    public Task<CatalogResponse> ListAsync(CancellationToken cancellationToken = default) =>
        RunAsync<CatalogResponse>(["list", "--json"], cancellationToken);

    public Task<DoctorResponse> DoctorAsync(CancellationToken cancellationToken = default) =>
        RunAsync<DoctorResponse>(["doctor", "--json"], cancellationToken);

    public Task<ContextResponse> ContextAsync(string target, CancellationToken cancellationToken = default) =>
        RunAsync<ContextResponse>(CreateContextArguments(target), cancellationToken);

    public Task<ShellActionResponse> ConnectAsync(string target, CancellationToken cancellationToken = default) =>
        RunAsync<ShellActionResponse>(["connect", target, "--json"], cancellationToken);

    public Task<DesktopActionResponse> OpenAsync(string target, string? routeAdapter = null, CancellationToken cancellationToken = default) =>
        RunAsync<DesktopActionResponse>(CreateOpenArguments(target, routeAdapter), cancellationToken);

    public Task<DesktopActionResponse> SaveDesktopCredentialAndOpenAsync(
        string target,
        string credential,
        string? routeAdapter = null,
        CancellationToken cancellationToken = default) =>
        RunAsync<DesktopActionResponse>(CreateCredentialOpenArguments(target, routeAdapter), cancellationToken, credential);

    private async Task<T> RunAsync<T>(IReadOnlyList<string> arguments, CancellationToken cancellationToken, string? standardInput = null)
    {
        if (!File.Exists(_executablePath))
        {
            throw new FileNotFoundException("PF Remote CLI is not present beside Center.", _executablePath);
        }

        ProcessStartInfo startInfo = CreateStartInfo(_executablePath, arguments);
        startInfo.RedirectStandardInput = standardInput is not null;

        using var process = Process.Start(startInfo) ?? throw new InvalidOperationException("PF Remote CLI did not start.");
        if (standardInput is not null)
        {
            await process.StandardInput.WriteLineAsync(standardInput.AsMemory(), cancellationToken);
            process.StandardInput.Close();
        }
        string output = await process.StandardOutput.ReadToEndAsync(cancellationToken);
        string error = await process.StandardError.ReadToEndAsync(cancellationToken);
        await process.WaitForExitAsync(cancellationToken);
        if (process.ExitCode != 0)
        {
            CliErrorResponse? failure = null;
            try
            {
                failure = JsonSerializer.Deserialize<CliErrorResponse>(error, JsonOptions);
            }
            catch (JsonException)
            {
            }
            throw failure is null
                ? new InvalidOperationException(string.IsNullOrWhiteSpace(error) ? "PF Remote CLI failed." : error.Trim())
                : new PfRemoteCliException(failure.Code, failure.Summary);
        }

        return JsonSerializer.Deserialize<T>(output, JsonOptions)
            ?? throw new InvalidDataException("PF Remote CLI returned an empty response.");
    }

    internal static ProcessStartInfo CreateStartInfo(string executablePath, IReadOnlyList<string> arguments)
    {
        var startInfo = new ProcessStartInfo
        {
            FileName = executablePath,
            CreateNoWindow = true,
            UseShellExecute = false,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            StandardOutputEncoding = Encoding.UTF8,
            StandardErrorEncoding = Encoding.UTF8,
        };
        foreach (var argument in arguments)
        {
            startInfo.ArgumentList.Add(argument);
        }
        return startInfo;
    }

    internal static IReadOnlyList<string> CreateContextArguments(string canonicalTarget) =>
        ["context", canonicalTarget, "--json"];

    internal static IReadOnlyList<string> CreateOpenArguments(string canonicalTarget, string? routeAdapter = null) =>
        RouteArguments("open", canonicalTarget, routeAdapter, credential: false);

    internal static IReadOnlyList<string> CreateCredentialOpenArguments(string canonicalTarget, string? routeAdapter = null) =>
        RouteArguments("open", canonicalTarget, routeAdapter, credential: true);

    private static List<string> RouteArguments(string command, string canonicalTarget, string? routeAdapter, bool credential)
    {
        var arguments = new List<string> { command, canonicalTarget };
        if (!string.IsNullOrWhiteSpace(routeAdapter))
        {
            arguments.Add("--route");
            arguments.Add(routeAdapter);
        }
        if (credential)
        {
            arguments.Add("--credential-stdin");
        }
        arguments.Add("--json");
        return arguments;
    }
}

internal sealed record CliErrorResponse(
    [property: System.Text.Json.Serialization.JsonPropertyName("code")] string Code,
    [property: System.Text.Json.Serialization.JsonPropertyName("summary")] string Summary);

internal sealed class PfRemoteCliException(string code, string message) : InvalidOperationException(message)
{
    public string Code { get; } = code;
}
