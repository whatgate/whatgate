using Avalonia.Controls;
using Avalonia.Interactivity;
using Avalonia.Markup.Xaml;
using WhatGate.Desktop.Services;
using WhatGate.Desktop.ViewModels;

namespace WhatGate.Desktop.Views;

public partial class MainWindow : Window
{
    private readonly MainWindowViewModel _viewModel;

    public MainWindow()
    {
        InitializeComponent();

        _viewModel = new MainWindowViewModel(
            new SettingsService(),
            new WhatGateProcessService(),
            new WhatGateApiClient());
        DataContext = _viewModel;

        _viewModel.CopyRequested += CopyToClipboardAsync;
        Opened += OnOpened;
        Closed += OnClosed;
    }

    private void InitializeComponent() => AvaloniaXamlLoader.Load(this);

    private async void OnOpened(object? sender, EventArgs eventArgs) =>
        await _viewModel.InitializeAsync();

    private async Task CopyToClipboardAsync(string text)
    {
        var clipboard = TopLevel.GetTopLevel(this)?.Clipboard;
        if (clipboard is not null)
        {
            await clipboard.SetTextAsync(text);
        }
    }

    private void OnClosed(object? sender, EventArgs eventArgs)
    {
        _viewModel.CopyRequested -= CopyToClipboardAsync;
        _viewModel.Dispose();
    }
}
