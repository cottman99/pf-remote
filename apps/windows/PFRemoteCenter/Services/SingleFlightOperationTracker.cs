namespace PFRemoteCenter.Services;

internal sealed class SingleFlightOperationTracker
{
	private readonly Lock _gate = new();
	private readonly HashSet<string> _activeKeys = new(StringComparer.Ordinal);

	internal bool TryBegin(string key)
	{
		ArgumentException.ThrowIfNullOrWhiteSpace(key);
		lock (_gate)
		{
			return _activeKeys.Add(key);
		}
	}

	internal void End(string key)
	{
		ArgumentException.ThrowIfNullOrWhiteSpace(key);
		lock (_gate)
		{
			_activeKeys.Remove(key);
		}
	}
}
