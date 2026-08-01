namespace WhatGate.Desktop.Services;

public static class AppPaths
{
    public static string DataDirectory { get; } = Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),
        "WhatGate");

    public static string RuntimeDirectory { get; } = Path.Combine(DataDirectory, "runtime");

    public static string SettingsFile { get; } = Path.Combine(DataDirectory, "settings.json");

    public static void EnsureDirectories()
    {
        Directory.CreateDirectory(DataDirectory);
        Directory.CreateDirectory(RuntimeDirectory);
    }
}
