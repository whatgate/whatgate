using System.Text.Json.Serialization;

namespace WhatGate.Desktop.Models;

public sealed class NodeStatus
{
    [JsonPropertyName("peerID")]
    public string PeerId { get; set; } = "";

    [JsonPropertyName("role")]
    public string Role { get; set; } = "";

    [JsonPropertyName("coordinator")]
    public string Coordinator { get; set; } = "";

    [JsonPropertyName("exitEnabled")]
    public bool ExitEnabled { get; set; }

    [JsonPropertyName("exitRegion")]
    public string ExitRegion { get; set; } = "";

    [JsonPropertyName("exitLoad")]
    public int ExitLoad { get; set; }

    [JsonPropertyName("toRegion")]
    public string ToRegion { get; set; } = "";

    [JsonPropertyName("trustScope")]
    public string TrustScope { get; set; } = "";

    [JsonPropertyName("groups")]
    public List<string> Groups { get; set; } = [];

    [JsonPropertyName("canManage")]
    public bool CanManage { get; set; }

    [JsonPropertyName("connectedExit")]
    public string ConnectedExit { get; set; } = "";

    [JsonPropertyName("socksAddr")]
    public string SocksAddress { get; set; } = "";

    [JsonPropertyName("canSwitch")]
    public bool CanSwitch { get; set; }

    [JsonPropertyName("canToggleExit")]
    public bool CanToggleExit { get; set; }

    [JsonPropertyName("needsSetup")]
    public bool NeedsSetup { get; set; }

    [JsonPropertyName("uptime")]
    public string Uptime { get; set; } = "";
}
