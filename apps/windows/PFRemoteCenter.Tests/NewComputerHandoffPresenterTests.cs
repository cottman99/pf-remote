using PFRemoteCenter.Presentation;

namespace PFRemoteCenter.Tests;

[TestClass]
public sealed class NewComputerHandoffPresenterTests
{
	private static readonly Dictionary<string, string> Resources = new(StringComparer.Ordinal)
	{
		["NewComputerHandoffMissingValue"] = "not provided",
		["NewComputerHandoffGoal"] = "Add a new computer to the current PF Remote.",
		["NewComputerHandoffInformationHeading"] = "User information",
		["NewComputerHandoffInformationHelp"] = "Confirm missing information safely on the new computer.",
		["NewComputerNameLabel"] = "Computer name",
		["NewComputerOperatingSystemLabel"] = "Operating system",
		["NewComputerUseLabel"] = "Intended use",
		["NewComputerRouteLabel"] = "Preferred route",
		["NewComputerInstallStateLabel"] = "Installation state",
		["NewComputerInstallLocationLabel"] = "Installation location",
		["NewComputerNotesLabel"] = "Notes",
		["NewComputerHandoffRequirementsHeading"] = "Required work",
		["NewComputerHandoffRequirementInspect"] = "Inspect current PF Remote safely.",
		["NewComputerHandoffRequirementInstall"] = "Install the matching client.",
		["NewComputerHandoffRequirementEnroll"] = "Enroll and authorize the device.",
		["NewComputerHandoffRequirementCapabilities"] = "Publish the intended capabilities.",
		["NewComputerHandoffRequirementRoutes"] = "Detect and validate eligible routes.",
		["NewComputerHandoffRequirementVerify"] = "Verify the visible result.",
		["NewComputerHandoffPrivacyHeading"] = "Privacy",
		["NewComputerHandoffPrivacyNoSecrets"] = "Do not place secrets in source control or reports.",
		["NewComputerHandoffPrivacyNoGlobalNetwork"] = "Do not change global networking.",
		["NewComputerHandoffPrivacyPreserve"] = "Preserve existing computers and rollback.",
		["NewComputerHandoffGuideReference"] = "Guide: docs/user/ADDING_A_COMPUTER.md",
		["NewComputerHandoffAcceptance"] = "Done when the computer appears and works.",
	};

	[TestMethod]
	public void CreateProducesSelfContainedAgentTaskAndKeepsBlankFieldsOptional()
	{
		var input = new NewComputerHandoffInput("Studio PC", "Windows", "Desktop and Agent", "Automatic", "Not installed", "", "Near the router");

		string result = NewComputerHandoffPresenter.Create(input, key => Resources[key]);

		StringAssert.StartsWith(result, "PF_REMOTE_ONBOARDING/1");
		StringAssert.Contains(result, "Computer name: Studio PC");
		StringAssert.Contains(result, "Installation location: not provided");
		StringAssert.Contains(result, "Detect and validate eligible routes.");
		StringAssert.Contains(result, "docs/user/ADDING_A_COMPUTER.md");
		Assert.IsFalse(result.Contains("gateway_url", StringComparison.OrdinalIgnoreCase));
		Assert.IsFalse(result.Contains("credential", StringComparison.OrdinalIgnoreCase));
	}

	[TestMethod]
	public void ContainsLikelySecretRejectsAssignmentsAndPrivateKeys()
	{
		Assert.IsTrue(NewComputerHandoffPresenter.ContainsLikelySecret(Input(notes: string.Concat("pass", "word", "=", "example"))));
		Assert.IsTrue(NewComputerHandoffPresenter.ContainsLikelySecret(Input(notes: string.Concat("令", "牌", "：", "示例"))));
		Assert.IsTrue(NewComputerHandoffPresenter.ContainsLikelySecret(Input(notes: string.Concat("-----BE", "GIN PRIVATE KEY-----"))));
		Assert.IsFalse(NewComputerHandoffPresenter.ContainsLikelySecret(Input(notes: "Agent 应在目标电脑上确认安装状态")));
	}

	private static NewComputerHandoffInput Input(string notes) =>
		new("", "Not sure", "Not sure", "Automatic", "Not sure", "", notes);
}
