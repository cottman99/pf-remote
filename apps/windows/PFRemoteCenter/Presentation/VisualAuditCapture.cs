#if DEBUG
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Media.Imaging;
using Windows.Graphics.Imaging;
using Windows.Storage;
using Windows.Storage.Streams;

namespace PFRemoteCenter.Presentation;

internal static class VisualAuditCapture
{
    private const string AuditPathVariable = "PFREMOTE_VISUAL_AUDIT_PATH";
    private static int _captureStarted;

    internal static async Task CaptureWhenRequestedAsync(FrameworkElement root)
    {
        string? requestedPath = Environment.GetEnvironmentVariable(AuditPathVariable);
        if (string.IsNullOrWhiteSpace(requestedPath) || Interlocked.Exchange(ref _captureStarted, 1) != 0)
        {
            return;
        }

        string fullPath = Path.GetFullPath(requestedPath);
        string? directory = Path.GetDirectoryName(fullPath);
        if (string.IsNullOrWhiteSpace(directory))
        {
            return;
        }

        Directory.CreateDirectory(directory);
        await Task.Delay(500);

        var bitmap = new RenderTargetBitmap();
        await bitmap.RenderAsync(root);
        IBuffer buffer = await bitmap.GetPixelsAsync();
        byte[] pixels = new byte[buffer.Length];
        using (DataReader reader = DataReader.FromBuffer(buffer))
        {
            reader.ReadBytes(pixels);
        }

        StorageFolder folder = await StorageFolder.GetFolderFromPathAsync(directory);
        StorageFile file = await folder.CreateFileAsync(Path.GetFileName(fullPath), CreationCollisionOption.ReplaceExisting);
        using IRandomAccessStream stream = await file.OpenAsync(FileAccessMode.ReadWrite);
        BitmapEncoder encoder = await BitmapEncoder.CreateAsync(BitmapEncoder.PngEncoderId, stream);
        encoder.SetPixelData(
            BitmapPixelFormat.Bgra8,
            BitmapAlphaMode.Premultiplied,
            (uint)bitmap.PixelWidth,
            (uint)bitmap.PixelHeight,
            96,
            96,
            pixels);
        await encoder.FlushAsync();
    }
}
#endif
