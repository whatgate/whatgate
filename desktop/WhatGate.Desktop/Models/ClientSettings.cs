namespace WhatGate.Desktop.Models;

public sealed class ClientSettings
{
    public string NetworkMode { get; set; } = "create";
    public string CoordinatorUrl { get; set; } = "http://127.0.0.1:8080";
    public string CoordinatorKey { get; set; } = "";
    public string Region { get; set; } = "JP";
    public string TrustScope { get; set; } = "conservative";
    public int WebPort { get; set; } = 7070;
    public int SocksPort { get; set; } = 1080;
    public int LocalCoordinatorPort { get; set; } = 8080;
}

public sealed record RegionOption(string Code, string Name, string Flag)
{
    public string DisplayName => $"{Flag}  {Name}";
}

public sealed record TrustModeOption(string Value, string Name, string Description);

public sealed record NetworkModeOption(string Value, string Name, string Description);

public sealed record LaunchOptions(
    string CoordinatorUrl,
    string CoordinatorKey,
    string InvitationCode,
    string Region,
    string TrustScope,
    int WebPort,
    int SocksPort,
    bool CreateNetwork,
    int LocalCoordinatorPort);
