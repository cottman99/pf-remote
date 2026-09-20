namespace PFRemoteCenter.Services;

internal static class BackgroundRecoveryPolicy
{
	internal static readonly TimeSpan HealthProbeInterval = TimeSpan.FromSeconds(5);
	internal static readonly TimeSpan MinimumRetryInterval = TimeSpan.FromSeconds(30);

	internal static bool ShouldAttempt(DateTimeOffset? lastAttempt, DateTimeOffset now) =>
		lastAttempt is null || now < lastAttempt.Value || now - lastAttempt.Value >= MinimumRetryInterval;
}
