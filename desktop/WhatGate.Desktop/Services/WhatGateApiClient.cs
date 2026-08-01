using System.Net.Http.Json;
using WhatGate.Desktop.Models;

namespace WhatGate.Desktop.Services;

public sealed class WhatGateApiClient : IDisposable
{
    private readonly HttpClient _httpClient = new()
    {
        Timeout = TimeSpan.FromSeconds(2),
    };

    public int WebPort { get; set; } = 7070;

    private string BaseUrl => $"http://127.0.0.1:{WebPort}";

    public async Task<NodeStatus?> GetStatusAsync(CancellationToken cancellationToken = default)
    {
        try
        {
            return await _httpClient.GetFromJsonAsync<NodeStatus>(
                $"{BaseUrl}/api/status",
                cancellationToken);
        }
        catch (HttpRequestException)
        {
            return null;
        }
        catch (TaskCanceledException) when (!cancellationToken.IsCancellationRequested)
        {
            return null;
        }
    }

    public Task SwitchRegionAsync(string region, CancellationToken cancellationToken = default) =>
        PostAsync("/api/switch", new { region }, cancellationToken);

    public Task ToggleExitAsync(bool enabled, CancellationToken cancellationToken = default) =>
        PostAsync("/api/exit", new { enabled }, cancellationToken);

    public Task JoinGroupAsync(
        string groupId,
        string secret,
        CancellationToken cancellationToken = default) =>
        PostAsync("/api/group/join", new { groupID = groupId, secret }, cancellationToken);

    public Task EndorseGroupAsync(
        string fromGroup,
        string toGroup,
        CancellationToken cancellationToken = default) =>
        PostAsync("/api/group/endorse", new { fromGroup, toGroup }, cancellationToken);

    public async Task<string> CreateInviteAsync(
        int maxUses,
        CancellationToken cancellationToken = default)
    {
        using var response = await _httpClient.PostAsJsonAsync(
            $"{BaseUrl}/api/invite/create",
            new { maxUses },
            cancellationToken);
        await ThrowForFailureAsync(response, cancellationToken);
        var result = await response.Content.ReadFromJsonAsync<InviteResponse>(cancellationToken);
        return !string.IsNullOrWhiteSpace(result?.Code)
            ? result.Code
            : throw new InvalidOperationException("邀请码生成失败，请稍后重试。");
    }

    private async Task PostAsync(string path, object body, CancellationToken cancellationToken)
    {
        using var response = await _httpClient.PostAsJsonAsync(
            $"{BaseUrl}{path}",
            body,
            cancellationToken);

        await ThrowForFailureAsync(response, cancellationToken);
    }

    private static async Task ThrowForFailureAsync(
        HttpResponseMessage response,
        CancellationToken cancellationToken)
    {
        if (response.IsSuccessStatusCode)
        {
            return;
        }

        var message = (await response.Content.ReadAsStringAsync(cancellationToken)).Trim();
        if (message.Contains("no exit in region", StringComparison.OrdinalIgnoreCase))
        {
            throw new InvalidOperationException("该地区暂时没有可用出口，请稍后重试或选择其他地区。");
        }

        throw new InvalidOperationException(
            string.IsNullOrWhiteSpace(message) ? "操作失败，请稍后重试。" : message);
    }

    private sealed class InviteResponse
    {
        public string Code { get; set; } = "";
    }

    public void Dispose() => _httpClient.Dispose();
}
