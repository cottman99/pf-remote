namespace PFRemoteCenter.Services;

// Await on the caller's UI context: only one native dialog may be shown at a time.
[System.Diagnostics.CodeAnalysis.SuppressMessage("Design", "CA1001", Justification = "Page-lifetime async-only semaphore; no WaitHandle is allocated and pending dialogs must not be disposed.")]
internal sealed class InteractionQueue
{
    private readonly SemaphoreSlim _gate = new(1, 1);

    internal async Task<T> RunAsync<T>(Func<Task<T>> action)
    {
        await _gate.WaitAsync();
        try { return await action(); }
        finally { _gate.Release(); }
    }
}
