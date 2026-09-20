using System.Text.RegularExpressions;

namespace PFRemoteCenter.Presentation;

internal sealed record NewComputerHandoffInput(
	string ComputerName,
	string OperatingSystem,
	string IntendedUse,
	string PreferredRoute,
	string InstallState,
	string InstallLocation,
	string Notes);

internal static partial class NewComputerHandoffPresenter
{
	[GeneratedRegex(@"(?i)(password|passwd|pwd|token|secret|api[_ -]?key|access[_ -]?key)\s*[:=]|(密码|口令|令牌|密钥)\s*[:：=]", RegexOptions.CultureInvariant)]
	private static partial Regex SecretAssignmentPattern();

	internal static bool ContainsLikelySecret(NewComputerHandoffInput input)
	{
		return new[]
		{
			input.ComputerName,
			input.InstallLocation,
			input.Notes,
		}.Any(value => SecretAssignmentPattern().IsMatch(value ?? string.Empty) ||
			(value ?? string.Empty).Contains("-----BEGIN", StringComparison.OrdinalIgnoreCase));
	}

	internal static string Create(NewComputerHandoffInput input, Func<string, string> resource)
	{
		ArgumentNullException.ThrowIfNull(input);
		ArgumentNullException.ThrowIfNull(resource);

		string missing = resource("NewComputerHandoffMissingValue");
		string Field(string labelKey, string value, int maximumLength = 160) =>
			$"- {resource(labelKey)}: {Normalize(value, missing, maximumLength)}";

		return string.Join(Environment.NewLine,
		[
			"PF_REMOTE_ONBOARDING/1",
			resource("NewComputerHandoffGoal"),
			string.Empty,
			$"## {resource("NewComputerHandoffInformationHeading")}",
			resource("NewComputerHandoffInformationHelp"),
			Field("NewComputerNameLabel", input.ComputerName),
			Field("NewComputerOperatingSystemLabel", input.OperatingSystem),
			Field("NewComputerUseLabel", input.IntendedUse),
			Field("NewComputerRouteLabel", input.PreferredRoute),
			Field("NewComputerInstallStateLabel", input.InstallState),
			Field("NewComputerInstallLocationLabel", input.InstallLocation, 240),
			Field("NewComputerNotesLabel", input.Notes, 500),
			string.Empty,
			$"## {resource("NewComputerHandoffRequirementsHeading")}",
			$"1. {resource("NewComputerHandoffRequirementInspect")}",
			$"2. {resource("NewComputerHandoffRequirementInstall")}",
			$"3. {resource("NewComputerHandoffRequirementEnroll")}",
			$"4. {resource("NewComputerHandoffRequirementCapabilities")}",
			$"5. {resource("NewComputerHandoffRequirementRoutes")}",
			$"6. {resource("NewComputerHandoffRequirementVerify")}",
			string.Empty,
			$"## {resource("NewComputerHandoffPrivacyHeading")}",
			$"- {resource("NewComputerHandoffPrivacyNoSecrets")}",
			$"- {resource("NewComputerHandoffPrivacyNoGlobalNetwork")}",
			$"- {resource("NewComputerHandoffPrivacyPreserve")}",
			string.Empty,
			resource("NewComputerHandoffGuideReference"),
			resource("NewComputerHandoffAcceptance"),
		]);
	}

	private static string Normalize(string? value, string missing, int maximumLength)
	{
		string normalized = Regex.Replace(value?.Trim() ?? string.Empty, @"\s+", " ");
		if (normalized.Length == 0)
		{
			return missing;
		}

		return normalized.Length <= maximumLength ? normalized : normalized[..maximumLength];
	}
}
