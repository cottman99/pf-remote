using System.IO;

using Microsoft.UI.Xaml;
using Microsoft.Windows.ApplicationModel.Resources;

namespace PFRemoteCenter;

public sealed partial class MainWindow : Window
{
    public MainWindow()
    {
        InitializeComponent();
        var resources = new ResourceLoader();
        Title = resources.GetString("CenterWindowTitle");
        AppTitleBar.Title = resources.GetString("AppTitleBarTitle");
        ExtendsContentIntoTitleBar = true;
        SetTitleBar(AppTitleBar);
        AppWindow.SetIcon(Path.Combine(AppContext.BaseDirectory, "Assets", "AppIcon.ico"));
        RootFrame.Navigate(typeof(MainPage));
    }
}
