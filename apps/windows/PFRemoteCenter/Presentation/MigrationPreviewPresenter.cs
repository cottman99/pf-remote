using System.Globalization;
using System.Text;

using PFRemoteCenter.Models;

namespace PFRemoteCenter.Presentation;

internal sealed record MigrationPreviewPresentation(string Summary, string Details);

internal static class MigrationPreviewPresenter
{
    public static MigrationPreviewPresentation Create(
        LegacyInventoryResponse inventory,
        CultureInfo culture,
        Func<string, string> resource)
    {
        if (!string.Equals(inventory.SchemaVersion, "pfremote.legacy-inventory/v1", StringComparison.Ordinal) ||
            inventory.Devices.Count == 0 || inventory.Devices.Any(device => device.Capabilities.Count == 0))
        {
            throw new InvalidDataException("PF Remote migration preview is incomplete.");
        }

        int actionCount = inventory.Devices.Sum(device => device.Capabilities.Count);
        string summary = string.Format(culture, resource("MigrationPreviewSummary"), inventory.Devices.Count, actionCount);
        var details = new StringBuilder();
        foreach (LegacyDevicePreview device in inventory.Devices)
        {
            details.AppendLine(device.DisplayName);
            foreach (LegacyCapabilityPreview capability in device.Capabilities)
            {
                string kind = capability.Kind switch
                {
                    "shell" => resource("MigrationShellLabel"),
                    "desktop" => resource("MigrationDesktopLabel"),
                    _ => throw new InvalidDataException("PF Remote migration preview contains an unsupported action."),
                };
                details.Append("  • ");
                details.AppendLine(string.Format(
                    culture,
                    resource("MigrationCapabilityLine"),
                    capability.DisplayName,
                    kind,
                    capability.PathCount));
            }
            details.AppendLine();
        }
        return new MigrationPreviewPresentation(summary, details.ToString().TrimEnd());
    }
}
