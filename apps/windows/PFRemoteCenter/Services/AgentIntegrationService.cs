using System.Diagnostics;
using System.Text;
using System.Text.Json;

namespace PFRemoteCenter.Services;

internal enum AgentIntegrationKind
{
    Unavailable,
    NotEnabled,
    Ready,
    UpdateAvailable,
    Conflict,
}

internal sealed record AgentIntegrationStatus(AgentIntegrationKind Kind);

internal sealed class AgentIntegrationService
{
    internal const string ServerName = "pf_remote";
    internal const string ManagedMarkerFileName = ".pfremote-managed.json";
    private const string ManagedMarkerSchema = "pfremote.codex-integration/v1";

    private static readonly JsonSerializerOptions WebJson = new(JsonSerializerDefaults.Web);
    private readonly string _bundledSkillRoot;
    private readonly string _installedSkillRoot;
    private readonly CodexCommand? _codex;

    internal AgentIntegrationService(
        string? bundledSkillRoot = null,
        string? userProfile = null,
        string? codexCommand = null)
    {
        _bundledSkillRoot = bundledSkillRoot ?? Path.Combine(AppContext.BaseDirectory, "skills", "pf-remote");
        string profile = userProfile ?? Environment.GetFolderPath(Environment.SpecialFolder.UserProfile);
        _installedSkillRoot = Path.Combine(profile, ".agents", "skills", "pf-remote");
        _codex = ResolveCodexCommand(codexCommand);
    }

    internal string InstalledMcpWrapperPath => Path.Combine(_installedSkillRoot, "scripts", "pfremote-mcp.ps1");

    internal async Task<AgentIntegrationStatus> GetStatusAsync()
    {
        if (_codex is null || !HasCompleteBundledSkill())
        {
            return new(AgentIntegrationKind.Unavailable);
        }

        ProcessResult registration = await RunCodexAsync(["mcp", "get", ServerName, "--json"]);
        if (registration.ExitCode != 0)
        {
            if (Directory.Exists(_installedSkillRoot) && !IsManagedSkill(_installedSkillRoot))
            {
                return new(AgentIntegrationKind.Conflict);
            }
            return new(AgentIntegrationKind.NotEnabled);
        }

        if (!RegistrationMatches(registration.StandardOutput, InstalledMcpWrapperPath))
        {
            return new(AgentIntegrationKind.Conflict);
        }

        return IsManagedSkillCurrent()
            ? new(AgentIntegrationKind.Ready)
            : new(AgentIntegrationKind.UpdateAvailable);
    }

    internal async Task EnableAsync()
    {
        AgentIntegrationStatus status = await GetStatusAsync();
        if (status.Kind == AgentIntegrationKind.Unavailable)
        {
            throw new InvalidOperationException("Codex integration is unavailable.");
        }
        if (status.Kind == AgentIntegrationKind.Conflict)
        {
            throw new InvalidOperationException("A different pf_remote MCP registration already exists.");
        }

        InstallManagedSkill();
        if (status.Kind is AgentIntegrationKind.Ready or AgentIntegrationKind.UpdateAvailable)
        {
            ProcessResult remove = await RunCodexAsync(["mcp", "remove", ServerName]);
            if (remove.ExitCode != 0)
            {
                throw new InvalidOperationException("The existing PF Remote Codex entry could not be updated.");
            }
        }

        ProcessResult add = await RunCodexAsync([
            "mcp", "add", ServerName, "--",
            "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
            "-File", InstalledMcpWrapperPath,
        ]);
        if (add.ExitCode != 0)
        {
            throw new InvalidOperationException("PF Remote could not be enabled in Codex.");
        }
    }

    internal async Task DisableAsync()
    {
        AgentIntegrationStatus status = await GetStatusAsync();
        if (status.Kind is not (AgentIntegrationKind.Ready or AgentIntegrationKind.UpdateAvailable))
        {
            throw new InvalidOperationException("PF Remote does not own the current Codex entry.");
        }

        ProcessResult remove = await RunCodexAsync(["mcp", "remove", ServerName]);
        if (remove.ExitCode != 0)
        {
            throw new InvalidOperationException("PF Remote could not be removed from Codex.");
        }

        if (IsManagedSkill(_installedSkillRoot))
        {
            Directory.Delete(_installedSkillRoot, recursive: true);
        }
    }

    internal static IReadOnlyList<string> ExpectedMcpArguments(string wrapperPath) =>
    [
        "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", wrapperPath,
    ];

    internal static bool RegistrationMatches(string json, string expectedWrapperPath)
    {
        try
        {
            using JsonDocument document = JsonDocument.Parse(json);
            JsonElement root = document.RootElement;
            if (!root.TryGetProperty("name", out JsonElement name) || name.GetString() != ServerName ||
                !root.TryGetProperty("transport", out JsonElement transport) ||
                !transport.TryGetProperty("type", out JsonElement type) || type.GetString() != "stdio" ||
                !transport.TryGetProperty("command", out JsonElement command) ||
                !string.Equals(command.GetString(), "powershell.exe", StringComparison.OrdinalIgnoreCase) ||
                !transport.TryGetProperty("args", out JsonElement args) || args.ValueKind != JsonValueKind.Array)
            {
                return false;
            }

            string[] actual = args.EnumerateArray().Select(value => value.GetString() ?? string.Empty).ToArray();
            string[] expected = ExpectedMcpArguments(expectedWrapperPath).ToArray();
            if (actual.Length != expected.Length)
            {
                return false;
            }
            for (int index = 0; index < expected.Length; index++)
            {
                StringComparison comparison = index == expected.Length - 1
                    ? StringComparison.OrdinalIgnoreCase
                    : StringComparison.Ordinal;
                if (!string.Equals(actual[index], expected[index], comparison))
                {
                    return false;
                }
            }
            return true;
        }
        catch (JsonException)
        {
            return false;
        }
    }

    internal static bool IsManagedSkill(string skillRoot)
    {
        string markerPath = Path.Combine(skillRoot, ManagedMarkerFileName);
        try
        {
            using JsonDocument document = JsonDocument.Parse(File.ReadAllText(markerPath));
            return document.RootElement.TryGetProperty("schema_version", out JsonElement schema) &&
                schema.GetString() == ManagedMarkerSchema &&
                document.RootElement.TryGetProperty("product", out JsonElement product) &&
                product.GetString() == "PF Remote";
        }
        catch (IOException)
        {
            return false;
        }
        catch (UnauthorizedAccessException)
        {
            return false;
        }
        catch (JsonException)
        {
            return false;
        }
    }

    private bool HasCompleteBundledSkill() =>
        File.Exists(Path.Combine(_bundledSkillRoot, "SKILL.md")) &&
        File.Exists(Path.Combine(_bundledSkillRoot, "scripts", "pfremote.ps1")) &&
        File.Exists(Path.Combine(_bundledSkillRoot, "scripts", "pfremote-mcp.ps1"));

    private bool IsManagedSkillCurrent()
    {
        if (!IsManagedSkill(_installedSkillRoot))
        {
            return false;
        }
        string[] relativeFiles =
        [
            "SKILL.md",
            Path.Combine("scripts", "pfremote.ps1"),
            Path.Combine("scripts", "pfremote-mcp.ps1"),
            Path.Combine("agents", "openai.yaml"),
        ];
        return relativeFiles.All(relative => FilesEqual(
            Path.Combine(_bundledSkillRoot, relative),
            Path.Combine(_installedSkillRoot, relative)));
    }

    private void InstallManagedSkill()
    {
        string parent = Path.GetDirectoryName(_installedSkillRoot)
            ?? throw new InvalidOperationException("The Codex skill directory is unavailable.");
        Directory.CreateDirectory(parent);
        if (Directory.Exists(_installedSkillRoot) && !IsManagedSkill(_installedSkillRoot))
        {
            throw new InvalidOperationException("A user-managed PF Remote skill already exists.");
        }

        string stage = Path.Combine(parent, $".pf-remote-installing-{Guid.NewGuid():N}");
        string backup = Path.Combine(parent, $".pf-remote-previous-{Guid.NewGuid():N}");
        CopyDirectory(_bundledSkillRoot, stage);
        File.WriteAllText(
            Path.Combine(stage, ManagedMarkerFileName),
            JsonSerializer.Serialize(new { schema_version = ManagedMarkerSchema, product = "PF Remote" }, WebJson),
            new UTF8Encoding(encoderShouldEmitUTF8Identifier: false));

        bool movedPrevious = false;
        try
        {
            if (Directory.Exists(_installedSkillRoot))
            {
                Directory.Move(_installedSkillRoot, backup);
                movedPrevious = true;
            }
            Directory.Move(stage, _installedSkillRoot);
            if (movedPrevious)
            {
                Directory.Delete(backup, recursive: true);
            }
        }
        catch
        {
            if (!Directory.Exists(_installedSkillRoot) && movedPrevious && Directory.Exists(backup))
            {
                Directory.Move(backup, _installedSkillRoot);
            }
            throw;
        }
        finally
        {
            if (Directory.Exists(stage))
            {
                Directory.Delete(stage, recursive: true);
            }
        }
    }

    private async Task<ProcessResult> RunCodexAsync(IReadOnlyList<string> arguments)
    {
        if (_codex is null)
        {
            return new(-1, string.Empty, string.Empty);
        }
        var startInfo = new ProcessStartInfo
        {
            FileName = _codex.FileName,
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            StandardOutputEncoding = Encoding.UTF8,
            StandardErrorEncoding = Encoding.UTF8,
        };
        foreach (string prefix in _codex.PrefixArguments)
        {
            startInfo.ArgumentList.Add(prefix);
        }
        foreach (string argument in arguments)
        {
            startInfo.ArgumentList.Add(argument);
        }

        using Process process = Process.Start(startInfo)
            ?? throw new InvalidOperationException("Codex could not be started.");
        Task<string> standardOutput = process.StandardOutput.ReadToEndAsync();
        Task<string> standardError = process.StandardError.ReadToEndAsync();
        await process.WaitForExitAsync();
        return new(process.ExitCode, await standardOutput, await standardError);
    }

    private static CodexCommand? ResolveCodexCommand(string? explicitCommand)
    {
        string? path = explicitCommand;
        if (string.IsNullOrWhiteSpace(path))
        {
            // The Microsoft Store package also exposes an ACL-restricted
            // internal codex.exe. Prefer the normal user command shim that is
            // already usable from the user's Agent environment.
            path = FindOnPath("codex.ps1") ?? FindOnPath("codex.exe");
        }
        if (string.IsNullOrWhiteSpace(path) || !File.Exists(path))
        {
            return null;
        }
        if (string.Equals(Path.GetExtension(path), ".ps1", StringComparison.OrdinalIgnoreCase))
        {
            return new("powershell.exe", ["-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", path]);
        }
        return new(path, []);
    }

    private static string? FindOnPath(string fileName)
    {
        string? value = Environment.GetEnvironmentVariable("PATH");
        if (string.IsNullOrWhiteSpace(value))
        {
            return null;
        }
        foreach (string entry in value.Split(Path.PathSeparator, StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries))
        {
            try
            {
                string candidate = Path.Combine(entry.Trim('"'), fileName);
                if (File.Exists(candidate))
                {
                    return candidate;
                }
            }
            catch (ArgumentException)
            {
                // Ignore malformed entries inherited from unrelated software.
            }
        }
        return null;
    }

    private static void CopyDirectory(string source, string destination)
    {
        Directory.CreateDirectory(destination);
        foreach (string file in Directory.EnumerateFiles(source))
        {
            File.Copy(file, Path.Combine(destination, Path.GetFileName(file)));
        }
        foreach (string directory in Directory.EnumerateDirectories(source))
        {
            CopyDirectory(directory, Path.Combine(destination, Path.GetFileName(directory)));
        }
    }

    private static bool FilesEqual(string left, string right)
    {
        if (!File.Exists(left) || !File.Exists(right))
        {
            return false;
        }
        var leftInfo = new FileInfo(left);
        var rightInfo = new FileInfo(right);
        return leftInfo.Length == rightInfo.Length && File.ReadAllBytes(left).SequenceEqual(File.ReadAllBytes(right));
    }

    private sealed record CodexCommand(string FileName, IReadOnlyList<string> PrefixArguments);
    private sealed record ProcessResult(int ExitCode, string StandardOutput, string StandardError);
}
