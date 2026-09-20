using System.Security.Cryptography;
using System.Text;

namespace PFRemoteCenter.Services;

// The owning UI thread holds the mutex for the lifetime of the application.
// This channel carries only a request to show the window, never action arguments.
internal sealed class SingleInstanceService : IDisposable
{
    private readonly Mutex _ownership;
    private readonly EventWaitHandle _activation;
    private RegisteredWaitHandle? _listener;
    private bool _disposed;

    internal bool IsPrimary { get; }

    internal SingleInstanceService(string scope)
    {
        string key = Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(scope)));
        // Create the event first: an activation during primary startup stays pending.
        _activation = new EventWaitHandle(false, EventResetMode.AutoReset, $"Local\\PFRemote.Center.Show.{key}");
        _ownership = new Mutex(false, $"Local\\PFRemote.Center.Owner.{key}");
        try
        {
            IsPrimary = _ownership.WaitOne(0);
        }
        catch (AbandonedMutexException)
        {
            IsPrimary = true;
        }
    }

    internal static bool ShouldShow(IEnumerable<string> arguments) =>
        !arguments.Any(argument => string.Equals(argument, "--background", StringComparison.OrdinalIgnoreCase));

    internal void RequestActivation() => _activation.Set();

    internal void Listen(Action activate)
    {
        if (!IsPrimary || _listener is not null)
            throw new InvalidOperationException("Only the primary instance can register activation once.");
        _listener = ThreadPool.RegisterWaitForSingleObject(_activation,
            (_, timedOut) => { if (!timedOut) activate(); }, null, Timeout.Infinite, false);
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        _listener?.Unregister(null);
        if (IsPrimary) _ownership.ReleaseMutex();
        _ownership.Dispose();
        _activation.Dispose();
    }
}
