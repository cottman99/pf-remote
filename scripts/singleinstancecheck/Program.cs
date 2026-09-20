using System.Diagnostics;
using PFRemoteCenter.Services;

if (args.Length > 0)
{
    using var instance = new SingleInstanceService(args[1]);
    if (args[0] == "hold")
    {
        Console.WriteLine(instance.IsPrimary ? "primary" : "secondary");
        Console.Out.Flush();
        Thread.Sleep(Timeout.Infinite);
    }
    if (instance.IsPrimary) return 2;
    if (SingleInstanceService.ShouldShow(args.Skip(2))) instance.RequestActivation();
    return 0;
}

string scope = "synthetic-single-instance-" + Guid.NewGuid().ToString("N");
Process Child(string mode, string childScope, bool background = false)
{
    var start = new ProcessStartInfo(Environment.ProcessPath!)
    {
        UseShellExecute = false, CreateNoWindow = true, RedirectStandardOutput = true,
    };
    start.ArgumentList.Add(mode);
    start.ArgumentList.Add(childScope);
    if (background) start.ArgumentList.Add("--background");
    return Process.Start(start)!;
}
void Check(bool condition, string message)
{
    if (!condition) throw new InvalidOperationException(message);
}
void Finish(Process child)
{
    using (child)
    {
        if (!child.WaitForExit(10000)) { child.Kill(); throw new InvalidOperationException("Secondary did not exit."); }
        Check(child.ExitCode == 0, "Secondary created another primary.");
    }
}

int activations = 0;
using (var primary = new SingleInstanceService(scope))
{
    Check(primary.IsPrimary, "First launch did not own the instance.");
    // A launch arriving before the window/listener is ready must not be lost.
    Finish(Child("launch", scope));
    primary.Listen(() => Interlocked.Increment(ref activations));
    Check(SpinWait.SpinUntil(() => Volatile.Read(ref activations) == 1, 5000), "Startup activation was lost.");

    var concurrent = Enumerable.Range(0, 12).Select(_ => Child("launch", scope, true)).ToArray();
    foreach (var child in concurrent) Finish(child);
    Thread.Sleep(150);
    Check(activations == 1, "Background startup requested foreground activation.");

    for (int i = 0; i < 3; i++)
    {
        int expected = activations + 1;
        Finish(Child("launch", scope));
        Check(SpinWait.SpinUntil(() => Volatile.Read(ref activations) == expected, 5000), "Explicit launch was not redirected.");
    }
    using var other = new SingleInstanceService(scope + "-other-session");
    Check(other.IsPrimary, "Another session was incorrectly blocked.");
}
using (var restarted = new SingleInstanceService(scope))
    Check(restarted.IsPrimary, "Normal exit retained instance ownership.");

using (var crashed = Child("hold", scope + "-crash"))
{
    try
    {
        var ready = crashed.StandardOutput.ReadLineAsync();
        Check(ready.Wait(10000) && ready.Result == "primary", "Crash fixture did not start.");
        crashed.Kill();
        crashed.WaitForExit();
        using var recovered = new SingleInstanceService(scope + "-crash");
        Check(recovered.IsPrimary, "Crash retained instance ownership.");
    }
    finally { if (!crashed.HasExited) crashed.Kill(); }
}
Console.WriteLine("Single instance passed: concurrent launches, pending activation, silent background, explicit activation, session isolation, exit and crash recovery.");
return 0;
