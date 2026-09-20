using System.Diagnostics;
using System.Runtime.Versioning;

using Microsoft.Win32;

namespace PFRemoteCenter.Services;

internal sealed class LegacyCenterLauncher
{
    private const string LegacyDisplayName = "PF Remote Center";
    private const string LegacyExecutableName = "PFRemoteCenter.exe";
	private readonly string _currentApplicationDirectory = AppContext.BaseDirectory;

    public bool IsAvailable => FindExecutable(_currentApplicationDirectory) is not null;

    public void Open()
    {
        string executable = FindExecutable(_currentApplicationDirectory)
            ?? throw new FileNotFoundException("The old PF Remote Center installation is unavailable.");
        _ = Process.Start(new ProcessStartInfo(executable) { UseShellExecute = true })
            ?? throw new InvalidOperationException("The old PF Remote Center did not start.");
    }

    private static string? FindExecutable(string currentApplicationDirectory)
    {
		if (!OperatingSystem.IsWindows())
		{
			return null;
		}
        List<string> candidates = [];
        foreach ((RegistryHive hive, RegistryView view) in RegistryLocations())
        {
            try
            {
                using RegistryKey baseKey = RegistryKey.OpenBaseKey(hive, view);
                using RegistryKey? uninstall = baseKey.OpenSubKey(@"SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall", false);
                if (uninstall is null)
                {
                    continue;
                }
                foreach (string subKeyName in uninstall.GetSubKeyNames())
                {
                    using RegistryKey? product = uninstall.OpenSubKey(subKeyName, false);
					if (product is null || !string.Equals(product.GetValue("DisplayName") as string, LegacyDisplayName, StringComparison.OrdinalIgnoreCase))
                    {
                        continue;
                    }
                    if (product.GetValue("DisplayIcon") is string displayIcon)
                    {
                        candidates.Add(ParseDisplayIcon(displayIcon));
                    }
                    if (product.GetValue("InstallLocation") is string installLocation && !string.IsNullOrWhiteSpace(installLocation))
                    {
                        candidates.Add(Path.Combine(installLocation, "app", LegacyExecutableName));
                        candidates.Add(Path.Combine(installLocation, LegacyExecutableName));
                    }
                }
            }
            catch (Exception failure) when (failure is UnauthorizedAccessException or IOException or System.Security.SecurityException)
            {
                // A registry view can be unavailable without making another
                // registered ordinary-user installation unsafe to use.
            }
        }
        string programFiles = Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles);
        if (!string.IsNullOrWhiteSpace(programFiles))
        {
            candidates.Add(Path.Combine(programFiles, LegacyDisplayName, "app", LegacyExecutableName));
        }
        return SelectExistingExecutable(candidates, currentApplicationDirectory);
    }

    internal static string? SelectExistingExecutable(
        IEnumerable<string> candidates,
        string currentApplicationDirectory,
        Func<string, bool>? exists = null)
    {
        exists ??= File.Exists;
        string currentDirectory = Path.GetFullPath(currentApplicationDirectory)
            .TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar) + Path.DirectorySeparatorChar;
        foreach (string candidate in candidates)
        {
            if (string.IsNullOrWhiteSpace(candidate))
            {
                continue;
            }
            string fullPath;
            try
            {
                fullPath = Path.GetFullPath(candidate.Trim());
            }
            catch (Exception failure) when (failure is ArgumentException or NotSupportedException or PathTooLongException)
            {
                continue;
            }
            if (!string.Equals(Path.GetFileName(fullPath), LegacyExecutableName, StringComparison.OrdinalIgnoreCase) ||
                fullPath.StartsWith(currentDirectory, StringComparison.OrdinalIgnoreCase) ||
                !exists(fullPath))
            {
                continue;
            }
            return fullPath;
        }
        return null;
    }

    internal static string ParseDisplayIcon(string value)
    {
        string candidate = value.Trim();
        int resourceSeparator = candidate.LastIndexOf(',');
        if (resourceSeparator > 0 && int.TryParse(candidate[(resourceSeparator + 1)..], out _))
        {
            candidate = candidate[..resourceSeparator];
        }
        return candidate.Trim().Trim('"');
    }

	[SupportedOSPlatform("windows")]
    private static IEnumerable<(RegistryHive Hive, RegistryView View)> RegistryLocations()
    {
        yield return (RegistryHive.CurrentUser, RegistryView.Registry64);
        yield return (RegistryHive.CurrentUser, RegistryView.Registry32);
        yield return (RegistryHive.LocalMachine, RegistryView.Registry64);
        yield return (RegistryHive.LocalMachine, RegistryView.Registry32);
    }
}
