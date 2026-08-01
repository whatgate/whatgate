using System.Text.Json;
using WhatGate.Desktop.Models;

namespace WhatGate.Desktop.Services;

public sealed class SettingsService
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        WriteIndented = true,
    };

    public async Task<ClientSettings> LoadAsync()
    {
        AppPaths.EnsureDirectories();
        if (!File.Exists(AppPaths.SettingsFile))
        {
            return new ClientSettings();
        }

        try
        {
            await using var stream = File.OpenRead(AppPaths.SettingsFile);
            return await JsonSerializer.DeserializeAsync<ClientSettings>(stream, JsonOptions)
                ?? new ClientSettings();
        }
        catch
        {
            return new ClientSettings();
        }
    }

    public async Task SaveAsync(ClientSettings settings)
    {
        AppPaths.EnsureDirectories();
        var tempPath = AppPaths.SettingsFile + ".tmp";
        await using (var stream = File.Create(tempPath))
        {
            await JsonSerializer.SerializeAsync(stream, settings, JsonOptions);
        }

        File.Move(tempPath, AppPaths.SettingsFile, true);
    }
}
