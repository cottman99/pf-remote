using System.ComponentModel;
using System.Runtime.InteropServices;

using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Microsoft.Windows.ApplicationModel.Resources;

namespace PFRemoteCenter.Services;

internal sealed class TrayIconService : IDisposable
{
    private const int WindowProcedureIndex = -4;
    private const uint CallbackMessage = 0x8001;
    private const uint LeftButtonDoubleClick = 0x0203;
    private const uint RightButtonUp = 0x0205;
    private const uint NotifyAdd = 0;
    private const uint NotifyDelete = 2;
    private const uint NotifyMessage = 1;
    private const uint NotifyIcon = 2;
    private const uint NotifyTip = 4;
    private const uint ImageIcon = 1;
    private const uint LoadFromFile = 0x0010;
    private const uint DefaultSize = 0x0040;
    private const uint MenuString = 0;
    private const uint MenuSeparator = 0x0800;
    private const uint ReturnCommand = 0x0100;
    private const uint RightButton = 0x0002;
    private const uint OpenCommand = 1;
    private const uint ExitCommand = 2;

    private readonly Window _window;
    private readonly nint _windowHandle;
    private readonly nint _iconHandle;
    private readonly WindowProcedure _windowProcedure;
    private readonly nint _previousWindowProcedure;
    private readonly ResourceLoader _resources = new();
    private bool _exitRequested;
    private bool _disposed;

    internal TrayIconService(Window window)
    {
        _window = window;
        _windowHandle = WinRT.Interop.WindowNative.GetWindowHandle(window);
        _windowProcedure = ProcessWindowMessage;
        _previousWindowProcedure = SetWindowLongPtr(_windowHandle, WindowProcedureIndex, Marshal.GetFunctionPointerForDelegate(_windowProcedure));
        if (_previousWindowProcedure == 0)
        {
            throw new Win32Exception(Marshal.GetLastWin32Error());
        }

        _iconHandle = LoadImage(0, Path.Combine(AppContext.BaseDirectory, "Assets", "AppIcon.ico"), ImageIcon, 0, 0, LoadFromFile | DefaultSize);
        if (_iconHandle == 0)
        {
            throw new Win32Exception(Marshal.GetLastWin32Error());
        }

        NotifyIconData data = CreateNotifyIconData();
        if (!ShellNotifyIcon(NotifyAdd, ref data))
        {
            throw new Win32Exception(Marshal.GetLastWin32Error());
        }
        _window.AppWindow.Closing += OnWindowClosing;
    }

    public void Dispose()
    {
        if (_disposed)
        {
            return;
        }
        _disposed = true;
        NotifyIconData data = CreateNotifyIconData();
        ShellNotifyIcon(NotifyDelete, ref data);
        SetWindowLongPtr(_windowHandle, WindowProcedureIndex, _previousWindowProcedure);
        if (_iconHandle != 0)
        {
            DestroyIcon(_iconHandle);
        }
    }

    private void OnWindowClosing(AppWindow sender, AppWindowClosingEventArgs args)
    {
        if (_exitRequested)
        {
            Dispose();
            return;
        }
        args.Cancel = true;
        sender.Hide();
    }

    private nint ProcessWindowMessage(nint window, uint message, nint wParam, nint lParam)
    {
        if (message == CallbackMessage)
        {
            uint mouseMessage = unchecked((uint)lParam.ToInt64());
            if (mouseMessage == LeftButtonDoubleClick)
            {
                ShowWindow();
                return 0;
            }
            if (mouseMessage == RightButtonUp)
            {
                ShowContextMenu();
                return 0;
            }
        }
        return CallWindowProc(_previousWindowProcedure, window, message, wParam, lParam);
    }

    private void ShowWindow()
    {
        if (_window.AppWindow.Presenter is OverlappedPresenter presenter &&
            presenter.State == OverlappedPresenterState.Minimized)
            presenter.Restore();
        _window.AppWindow.Show();
        _window.Activate();
    }

    private void ShowContextMenu()
    {
        nint menu = CreatePopupMenu();
        if (menu == 0)
        {
            return;
        }
        try
        {
            AppendMenu(menu, MenuString, OpenCommand, _resources.GetString("TrayOpenLabel"));
            AppendMenu(menu, MenuSeparator, 0, null);
            AppendMenu(menu, MenuString, ExitCommand, _resources.GetString("TrayExitLabel"));
            GetCursorPos(out Point point);
            SetForegroundWindow(_windowHandle);
            uint command = TrackPopupMenu(menu, ReturnCommand | RightButton, point.X, point.Y, 0, _windowHandle, 0);
            if (command == OpenCommand)
            {
                ShowWindow();
            }
            else if (command == ExitCommand)
            {
                _exitRequested = true;
                _window.Close();
                Application.Current.Exit();
            }
        }
        finally
        {
            DestroyMenu(menu);
        }
    }

    private NotifyIconData CreateNotifyIconData() => new()
    {
        Size = (uint)Marshal.SizeOf<NotifyIconData>(),
        Window = _windowHandle,
        Id = 1,
        Flags = NotifyMessage | NotifyIcon | NotifyTip,
        CallbackMessage = CallbackMessage,
        Icon = _iconHandle,
        Tip = _resources.GetString("TrayTooltip"),
        Info = "",
        InfoTitle = "",
    };

    private delegate nint WindowProcedure(nint window, uint message, nint wParam, nint lParam);

    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    private struct NotifyIconData
    {
        internal uint Size;
        internal nint Window;
        internal uint Id;
        internal uint Flags;
        internal uint CallbackMessage;
        internal nint Icon;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 128)] internal string Tip;
        internal uint State;
        internal uint StateMask;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 256)] internal string Info;
        internal uint Version;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 64)] internal string InfoTitle;
        internal uint InfoFlags;
        internal Guid Item;
        internal nint BalloonIcon;
    }

    [StructLayout(LayoutKind.Sequential)]
    private readonly struct Point
    {
        internal readonly int X;
        internal readonly int Y;
    }

    [DllImport("user32.dll", EntryPoint = "SetWindowLongPtrW", SetLastError = true)]
    private static extern nint SetWindowLongPtr(nint window, int index, nint value);
    [DllImport("user32.dll")]
    private static extern nint CallWindowProc(nint previous, nint window, uint message, nint wParam, nint lParam);
    [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern nint LoadImage(nint instance, string name, uint type, int width, int height, uint load);
    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool DestroyIcon(nint icon);
    [DllImport("shell32.dll", CharSet = CharSet.Unicode, EntryPoint = "Shell_NotifyIconW", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool ShellNotifyIcon(uint message, ref NotifyIconData data);
    [DllImport("user32.dll")]
    private static extern nint CreatePopupMenu();
    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool AppendMenu(nint menu, uint flags, uint id, string? text);
    [DllImport("user32.dll")]
    private static extern uint TrackPopupMenu(nint menu, uint flags, int x, int y, int reserved, nint window, nint rectangle);
    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool DestroyMenu(nint menu);
    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GetCursorPos(out Point point);
    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetForegroundWindow(nint window);
}
