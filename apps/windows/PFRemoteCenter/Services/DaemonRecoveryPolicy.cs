namespace PFRemoteCenter.Services;

internal static class DaemonRecoveryPolicy
{
	internal static bool ShouldRecover(Exception exception) =>
		exception is PfRemoteCliException { Code: "DAEMON_UNAVAILABLE" };
}
