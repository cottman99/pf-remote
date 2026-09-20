using System.Text;

namespace PFRemoteCenter.Services;

internal static class ContextHandoffAudit
{
    internal static bool TryExport(string envelope)
    {
#if DEBUG
        string? path = Environment.GetEnvironmentVariable("PFREMOTE_CONTEXT_EXPORT_PATH");
        if (string.IsNullOrWhiteSpace(path))
        {
            return false;
        }
        string fullPath = Path.GetFullPath(path);
        string? directory = Path.GetDirectoryName(fullPath);
        if (string.IsNullOrWhiteSpace(directory))
        {
            throw new InvalidOperationException("PF Remote context audit path has no directory.");
        }
        Directory.CreateDirectory(directory);
        string temporary = fullPath + ".new";
        File.WriteAllText(temporary, envelope, new UTF8Encoding(false));
        File.Move(temporary, fullPath, true);
        return true;
#else
        return false;
#endif
    }
}
