using System.Collections.ObjectModel;
using Avalonia.Threading;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using WhatGate.Desktop.Models;
using WhatGate.Desktop.Services;

namespace WhatGate.Desktop.ViewModels;

public partial class MainWindowViewModel : ViewModelBase, IDisposable
{
    private readonly SettingsService _settingsService;
    private readonly WhatGateProcessService _processService;
    private readonly WhatGateApiClient _apiClient;
    private readonly CancellationTokenSource _pollCancellation = new();
    private bool _initialized;

    public MainWindowViewModel(
        SettingsService settingsService,
        WhatGateProcessService processService,
        WhatGateApiClient apiClient)
    {
        _settingsService = settingsService;
        _processService = processService;
        _apiClient = apiClient;

        Regions =
        [
            new("JP", "日本", "🇯🇵"),
            new("SG", "新加坡", "🇸🇬"),
            new("US", "美国", "🇺🇸"),
            new("HK", "中国香港", "🇭🇰"),
            new("GB", "英国", "🇬🇧"),
            new("DE", "德国", "🇩🇪"),
            new("AU", "澳大利亚", "🇦🇺"),
            new("CA", "加拿大", "🇨🇦"),
        ];

        TrustModes =
        [
            new("conservative", "仅信任圈", "优先选择已认证或同组节点，推荐普通用户使用"),
            new("open", "开放网络", "允许使用陌生出口，可用范围更广但信任要求更低"),
        ];

        NetworkModes =
        [
            new("create", "创建我的网络", "我是第一个使用者，由客户端自动创建网络，无需邀请码"),
            new("join", "加入已有网络", "使用管理员提供的协调器地址和邀请码加入"),
        ];

        selectedRegion = Regions[0];
        selectedTrustMode = TrustModes[0];
        selectedNetworkMode = NetworkModes[0];
        isCreateNetworkMode = true;

        _processService.LogReceived += OnLogReceived;
        _processService.ProcessExited += OnProcessExited;
    }

    public IReadOnlyList<RegionOption> Regions { get; }

    public IReadOnlyList<TrustModeOption> TrustModes { get; }

    public IReadOnlyList<NetworkModeOption> NetworkModes { get; }

    public IReadOnlyList<int> InviteUseOptions { get; } = [1, 5, 10, 25];

    public ObservableCollection<string> Logs { get; } = [];

    public ObservableCollection<string> Groups { get; } = [];

    public event Func<string, Task>? CopyRequested;

    [ObservableProperty]
    private string coordinatorUrl = "http://127.0.0.1:8080";

    [ObservableProperty]
    private string coordinatorKey = "";

    [ObservableProperty]
    private string invitationCode = "";

    [ObservableProperty]
    private NetworkModeOption selectedNetworkMode;

    [ObservableProperty]
    private bool isCreateNetworkMode;

    [ObservableProperty]
    private bool isJoinNetworkMode;

    [ObservableProperty]
    private int localCoordinatorPort = 8080;

    [ObservableProperty]
    private RegionOption selectedRegion;

    [ObservableProperty]
    private TrustModeOption selectedTrustMode;

    [ObservableProperty]
    private string pageTitle = "连接总览";

    [ObservableProperty]
    private bool isHomeVisible = true;

    [ObservableProperty]
    private bool isSetupVisible;

    [ObservableProperty]
    private bool isLogsVisible;

    [ObservableProperty]
    private bool isAdvancedVisible;

    [ObservableProperty]
    private bool isBusy;

    [ObservableProperty]
    private bool isOnline;

    [ObservableProperty]
    private bool isOwnedProcess;

    [ObservableProperty]
    private bool canSwitchRegion;

    [ObservableProperty]
    private bool canToggleExit;

    [ObservableProperty]
    private bool exitEnabled;

    [ObservableProperty]
    private bool coreFound;

    [ObservableProperty]
    private string statusText = "尚未连接";

    [ObservableProperty]
    private string statusDetail = "完成连接设置后即可开始使用";

    [ObservableProperty]
    private string statusColor = "#F59E0B";

    [ObservableProperty]
    private string primaryButtonText = "启动连接";

    [ObservableProperty]
    private string startButtonText = "创建并启动网络";

    [ObservableProperty]
    private string peerId = "—";

    [ObservableProperty]
    private string peerIdFull = "—";

    [ObservableProperty]
    private string connectedExit = "—";

    [ObservableProperty]
    private string socksAddress = "127.0.0.1:1080";

    [ObservableProperty]
    private string proxyTitle = "本机代理尚未启动";

    [ObservableProperty]
    private string proxyDetail = "连接成功后自动显示代理地址";

    [ObservableProperty]
    private bool canCopyProxy;

    [ObservableProperty]
    private string uptime = "—";

    [ObservableProperty]
    private string coordinatorDisplay = "—";

    [ObservableProperty]
    private string roleDisplay = "—";

    [ObservableProperty]
    private string trustModeDisplay = "—";

    [ObservableProperty]
    private string exitRegionDisplay = "未共享";

    [ObservableProperty]
    private string exitLoadDisplay = "—";

    [ObservableProperty]
    private string managementStatus = "连接后可管理信任圈";

    [ObservableProperty]
    private string shareCoordinatorUrl = "—";

    [ObservableProperty]
    private string generatedInvitationCode = "";

    [ObservableProperty]
    private bool hasGeneratedInvitation;

    [ObservableProperty]
    private bool canCreateInvites;

    [ObservableProperty]
    private int inviteMaxUses = 5;

    [ObservableProperty]
    private bool canManageGroups;

    [ObservableProperty]
    private bool hasGroups;

    [ObservableProperty]
    private string newGroupId = "";

    [ObservableProperty]
    private string groupSecret = "";

    [ObservableProperty]
    private string selectedFromGroup = "";

    [ObservableProperty]
    private string endorseTargetGroup = "";

    [ObservableProperty]
    private string activeRegion = "日本";

    [ObservableProperty]
    private string shareButtonText = "开启共享出口";

    [ObservableProperty]
    private string coreHint = "正在查找核心程序…";

    [ObservableProperty]
    private string notice = "";

    [ObservableProperty]
    private bool hasNotice;

    public async Task InitializeAsync()
    {
        if (_initialized)
        {
            return;
        }

        _initialized = true;
        var settings = await _settingsService.LoadAsync();
        SelectedNetworkMode = NetworkModes.FirstOrDefault(mode => mode.Value == settings.NetworkMode)
            ?? NetworkModes[0];
        CoordinatorUrl = settings.CoordinatorUrl;
        CoordinatorKey = settings.CoordinatorKey;
        SelectedRegion = Regions.FirstOrDefault(region => region.Code == settings.Region) ?? Regions[0];
        SelectedTrustMode = TrustModes.FirstOrDefault(mode => mode.Value == settings.TrustScope) ?? TrustModes[0];
        SocksAddress = $"127.0.0.1:{settings.SocksPort}";
        LocalCoordinatorPort = settings.LocalCoordinatorPort;
        ShareCoordinatorUrl = WhatGateProcessService.GetShareCoordinatorUrl(LocalCoordinatorPort);
        _apiClient.WebPort = settings.WebPort;

        var corePath = _processService.ResolveCorePath();
        var coordinatorPath = _processService.ResolveCoordinatorPath();
        CoreFound = corePath is not null;
        CoreHint = corePath is null
            ? "没有找到核心程序，请重新安装客户端"
            : coordinatorPath is null
                ? $"连接核心就绪 · {Path.GetFileName(corePath)}"
                : "连接核心与建网服务均已就绪";

        OnLogReceived("WhatGate 桌面客户端已就绪");
        if (corePath is not null)
        {
            OnLogReceived($"核心位置：{corePath}");
        }
        if (coordinatorPath is not null)
        {
            OnLogReceived($"建网服务位置：{coordinatorPath}");
        }

        await RefreshStatusAsync();
        _ = PollStatusAsync(_pollCancellation.Token);
    }

    [RelayCommand]
    private void ShowHome()
    {
        PageTitle = "连接总览";
        IsHomeVisible = true;
        IsSetupVisible = false;
        IsLogsVisible = false;
        IsAdvancedVisible = false;
    }

    [RelayCommand]
    private void ShowSetup()
    {
        PageTitle = "连接设置";
        IsHomeVisible = false;
        IsSetupVisible = true;
        IsLogsVisible = false;
        IsAdvancedVisible = false;
    }

    [RelayCommand]
    private void ShowLogs()
    {
        PageTitle = "运行记录";
        IsHomeVisible = false;
        IsSetupVisible = false;
        IsLogsVisible = true;
        IsAdvancedVisible = false;
    }

    [RelayCommand]
    private void ShowAdvanced()
    {
        PageTitle = "高级管理";
        IsHomeVisible = false;
        IsSetupVisible = false;
        IsLogsVisible = false;
        IsAdvancedVisible = true;
    }

    [RelayCommand]
    private async Task PrimaryActionAsync()
    {
        if (IsOnline)
        {
            ShowSetup();
            return;
        }

        await StartAsync();
    }

    [RelayCommand]
    private async Task StartAsync()
    {
        if (IsBusy || IsOwnedProcess)
        {
            return;
        }

        if (IsOnline)
        {
            Notice = "本机已有 WhatGate 核心正在运行，无需重复启动。";
            ShowHome();
            return;
        }

        Notice = "";
        var createNetwork = SelectedNetworkMode.Value == "create";
        if (createNetwork)
        {
            if (_processService.ResolveCoordinatorPath() is null)
            {
                Notice = "安装包中缺少建网服务，请重新安装完整客户端。";
                ShowSetup();
                return;
            }
            if (LocalCoordinatorPort is < 1024 or > 65535)
            {
                Notice = "建网端口应填写 1024 到 65535 之间的数字。";
                ShowSetup();
                return;
            }
            CoordinatorUrl = $"http://127.0.0.1:{LocalCoordinatorPort}";
            ShareCoordinatorUrl = WhatGateProcessService.GetShareCoordinatorUrl(LocalCoordinatorPort);
        }
        else if (!Uri.TryCreate(CoordinatorUrl.Trim(), UriKind.Absolute, out var coordinator)
                 || coordinator.Scheme is not ("http" or "https"))
        {
            Notice = "请填写正确的协调器地址，例如 https://example.com:8080";
            ShowSetup();
            return;
        }

        var identityPath = Path.Combine(AppPaths.DataDirectory, "identity.key");
        if (!createNetwork && !File.Exists(identityPath) && string.IsNullOrWhiteSpace(InvitationCode))
        {
            Notice = "首次连接需要填写邀请码。";
            ShowSetup();
            return;
        }

        IsBusy = true;
        PrimaryButtonText = "正在连接…";
        StatusText = "正在连接";
        StatusDetail = "正在启动安全网络核心";
        StatusColor = "#3B82F6";

        var settings = new ClientSettings
        {
            NetworkMode = SelectedNetworkMode.Value,
            CoordinatorUrl = CoordinatorUrl.Trim(),
            CoordinatorKey = CoordinatorKey.Trim(),
            Region = SelectedRegion.Code,
            TrustScope = SelectedTrustMode.Value,
            WebPort = _apiClient.WebPort,
            SocksPort = ParsePort(SocksAddress, 1080),
            LocalCoordinatorPort = LocalCoordinatorPort,
        };

        try
        {
            await _settingsService.SaveAsync(settings);
            await _processService.StartAsync(new LaunchOptions(
                settings.CoordinatorUrl,
                settings.CoordinatorKey,
                InvitationCode,
                settings.Region,
                settings.TrustScope,
                settings.WebPort,
                settings.SocksPort,
                createNetwork,
                settings.LocalCoordinatorPort));

            InvitationCode = "";
            IsOwnedProcess = true;

            for (var attempt = 0; attempt < 30; attempt++)
            {
                await Task.Delay(500);
                if (await RefreshStatusAsync())
                {
                    if (createNetwork)
                    {
                        await CreateInviteCoreAsync();
                        Notice = HasGeneratedInvitation
                            ? "新网络已创建，你已成为首位管理员，成员邀请码也已生成。"
                            : "新网络已创建，你已成为首位管理员。可在高级管理中生成成员邀请码。";
                        ShowHome();
                    }
                    else
                    {
                        Notice = "连接成功，本机代理已经可以使用。";
                        ShowHome();
                    }
                    return;
                }

                if (!_processService.IsRunning)
                {
                    break;
                }
            }

            Notice = "核心已启动，但尚未连接成功。请在运行记录中查看原因。";
            ShowLogs();
        }
        catch (Exception exception)
        {
            Notice = FriendlyError(exception);
            OnLogReceived($"启动失败：{exception.Message}");
            ShowSetup();
        }
        finally
        {
            IsBusy = false;
            PrimaryButtonText = IsOnline ? "连接正常" : "启动连接";
            IsOwnedProcess = _processService.IsRunning;
        }
    }

    [RelayCommand]
    private async Task StopAsync()
    {
        if (IsBusy || !IsOwnedProcess)
        {
            return;
        }

        IsBusy = true;
        try
        {
            await _processService.StopAsync();
            await RefreshStatusAsync();
            Notice = "连接已停止。";
        }
        catch (Exception exception)
        {
            Notice = FriendlyError(exception);
        }
        finally
        {
            IsBusy = false;
            IsOwnedProcess = _processService.IsRunning;
        }
    }

    [RelayCommand]
    private async Task SwitchRegionAsync()
    {
        if (IsBusy || !IsOnline || !CanSwitchRegion)
        {
            return;
        }

        IsBusy = true;
        try
        {
            await _apiClient.SwitchRegionAsync(SelectedRegion.Code);
            await RefreshStatusAsync();
            Notice = $"已切换到{SelectedRegion.Name}线路。";
        }
        catch (Exception exception)
        {
            Notice = FriendlyError(exception);
        }
        finally
        {
            IsBusy = false;
        }
    }

    [RelayCommand]
    private async Task ToggleShareAsync()
    {
        if (IsBusy || !IsOnline || !CanToggleExit)
        {
            return;
        }

        IsBusy = true;
        try
        {
            await _apiClient.ToggleExitAsync(!ExitEnabled);
            await RefreshStatusAsync();
            Notice = ExitEnabled ? "已开启共享出口。" : "已停止共享出口。";
        }
        catch (Exception exception)
        {
            Notice = FriendlyError(exception);
        }
        finally
        {
            IsBusy = false;
        }
    }

    [RelayCommand]
    private async Task CopyProxyAsync()
    {
        if (CanCopyProxy && CopyRequested is not null)
        {
            await CopyRequested.Invoke(SocksAddress);
            Notice = "代理地址已复制。";
        }
    }

    [RelayCommand]
    private async Task GenerateInviteAsync()
    {
        if (IsBusy || !IsOnline || !CanCreateInvites)
        {
            Notice = IsOnline ? "当前节点不能生成邀请码。" : "请先启动或加入 WhatGate 网络。";
            return;
        }

        IsBusy = true;
        try
        {
            await CreateInviteCoreAsync();
            Notice = $"已生成可供 {InviteMaxUses} 台设备使用的邀请码。";
        }
        catch (Exception exception)
        {
            Notice = FriendlyError(exception);
        }
        finally
        {
            IsBusy = false;
        }
    }

    private async Task CreateInviteCoreAsync()
    {
        GeneratedInvitationCode = await _apiClient.CreateInviteAsync(InviteMaxUses);
        HasGeneratedInvitation = true;
        OnLogReceived($"已生成成员邀请码 · 可使用 {InviteMaxUses} 次");
    }

    [RelayCommand]
    private async Task CopyInvitationAsync()
    {
        if (HasGeneratedInvitation && CopyRequested is not null)
        {
            await CopyRequested.Invoke(GeneratedInvitationCode);
            Notice = "邀请码已复制。";
        }
    }

    [RelayCommand]
    private async Task CopyConnectionInfoAsync()
    {
        if (HasGeneratedInvitation && CopyRequested is not null)
        {
            var text = $"WhatGate 协调器：{ShareCoordinatorUrl}{Environment.NewLine}邀请码：{GeneratedInvitationCode}";
            await CopyRequested.Invoke(text);
            Notice = "协调器地址和邀请码已复制，可直接发给新成员。";
        }
    }

    [RelayCommand]
    private async Task JoinGroupAsync()
    {
        if (IsBusy || !IsOnline || !CanManageGroups)
        {
            Notice = IsOnline ? "当前节点没有信任圈管理权限。" : "请先连接 WhatGate 网络。";
            return;
        }

        var groupId = NewGroupId.Trim();
        var secret = GroupSecret;
        if (string.IsNullOrWhiteSpace(groupId) || string.IsNullOrWhiteSpace(secret))
        {
            Notice = "请填写信任圈名称和口令。";
            return;
        }

        IsBusy = true;
        try
        {
            await _apiClient.JoinGroupAsync(groupId, secret);
            GroupSecret = "";
            NewGroupId = "";
            await RefreshStatusAsync();
            Notice = $"已加入信任圈“{groupId}”。";
            OnLogReceived($"已加入信任圈：{groupId}");
        }
        catch (Exception exception)
        {
            GroupSecret = "";
            Notice = FriendlyError(exception);
        }
        finally
        {
            IsBusy = false;
        }
    }

    [RelayCommand]
    private async Task EndorseGroupAsync()
    {
        if (IsBusy || !IsOnline || !CanManageGroups)
        {
            Notice = IsOnline ? "当前节点没有信任圈管理权限。" : "请先连接 WhatGate 网络。";
            return;
        }

        var fromGroup = SelectedFromGroup.Trim();
        var toGroup = EndorseTargetGroup.Trim();
        if (string.IsNullOrWhiteSpace(fromGroup) || string.IsNullOrWhiteSpace(toGroup))
        {
            Notice = "请选择担保方信任圈，并填写要认可的信任圈。";
            return;
        }

        if (string.Equals(fromGroup, toGroup, StringComparison.OrdinalIgnoreCase))
        {
            Notice = "担保方和被认可的信任圈不能相同。";
            return;
        }

        IsBusy = true;
        try
        {
            await _apiClient.EndorseGroupAsync(fromGroup, toGroup);
            EndorseTargetGroup = "";
            Notice = $"“{fromGroup}”已认可“{toGroup}”。";
            OnLogReceived($"信任圈认可：{fromGroup} → {toGroup}");
        }
        catch (Exception exception)
        {
            Notice = FriendlyError(exception);
        }
        finally
        {
            IsBusy = false;
        }
    }

    [RelayCommand]
    private void ClearLogs()
    {
        Logs.Clear();
        OnLogReceived("运行记录已清空");
    }

    private async Task PollStatusAsync(CancellationToken cancellationToken)
    {
        using var timer = new PeriodicTimer(TimeSpan.FromSeconds(3));
        try
        {
            while (await timer.WaitForNextTickAsync(cancellationToken))
            {
                await RefreshStatusAsync(cancellationToken);
            }
        }
        catch (OperationCanceledException)
        {
            // Normal application shutdown.
        }
    }

    private async Task<bool> RefreshStatusAsync(CancellationToken cancellationToken = default)
    {
        var status = await _apiClient.GetStatusAsync(cancellationToken);
        IsOwnedProcess = _processService.IsRunning;
        if (status is null)
        {
            IsOnline = false;
            CanSwitchRegion = false;
            CanToggleExit = false;
            ExitEnabled = false;
            StatusText = IsOwnedProcess ? "正在连接" : "尚未连接";
            StatusDetail = IsOwnedProcess ? "核心正在建立安全通道" : "完成连接设置后即可开始使用";
            StatusColor = IsOwnedProcess ? "#3B82F6" : "#F59E0B";
            PrimaryButtonText = IsOwnedProcess ? "正在连接…" : "启动连接";
            PeerId = "—";
            PeerIdFull = "—";
            ConnectedExit = "—";
            Uptime = "—";
            CoordinatorDisplay = "—";
            RoleDisplay = "—";
            TrustModeDisplay = "—";
            ExitRegionDisplay = "未共享";
            ExitLoadDisplay = "—";
            ManagementStatus = "连接后可管理信任圈";
            CanManageGroups = false;
            CanCreateInvites = false;
            Groups.Clear();
            HasGroups = false;
            SelectedFromGroup = "";
            ShareButtonText = "开启共享出口";
            ProxyTitle = "本机代理尚未启动";
            ProxyDetail = "连接成功后自动显示代理地址";
            CanCopyProxy = false;
            return false;
        }

        IsOnline = true;
        var hasExit = !string.IsNullOrWhiteSpace(status.ConnectedExit);
        (StatusText, StatusDetail, StatusColor) = ConnectionDescription(status, hasExit);
        PrimaryButtonText = "调整设置";
        PeerId = ShortId(status.PeerId);
        PeerIdFull = string.IsNullOrWhiteSpace(status.PeerId) ? "—" : status.PeerId;
        ConnectedExit = ShortId(status.ConnectedExit);
        CoordinatorDisplay = string.IsNullOrWhiteSpace(status.Coordinator) ? "—" : status.Coordinator;
        RoleDisplay = RoleDescription(status.Role);
        TrustModeDisplay = TrustDescription(status.TrustScope);
        ExitRegionDisplay = status.ExitEnabled
            ? RegionDescription(status.ExitRegion)
            : "未共享";
        ExitLoadDisplay = status.ExitEnabled ? $"{status.ExitLoad} 个活动连接" : "—";
        ManagementStatus = status.CanManage ? "信任圈管理可用" : "当前节点没有管理权限";
        CanManageGroups = status.CanManage;
        CanCreateInvites = status.CanManage;
        ShareCoordinatorUrl = IsCreateNetworkMode
            ? WhatGateProcessService.GetShareCoordinatorUrl(LocalCoordinatorPort)
            : string.IsNullOrWhiteSpace(status.Coordinator)
                ? CoordinatorUrl
                : status.Coordinator.Split(',', StringSplitOptions.RemoveEmptyEntries)[0].Trim();
        SynchronizeGroups(status.Groups);
        CanCopyProxy = hasExit && !string.IsNullOrWhiteSpace(status.SocksAddress);
        if (CanCopyProxy)
        {
            SocksAddress = status.SocksAddress;
            ProxyTitle = "本机代理已准备";
            ProxyDetail = $"SOCKS5 · {SocksAddress}";
        }
        else
        {
            ProxyTitle = "本机代理尚未启动";
            ProxyDetail = "连接到可用出口后自动开启";
        }
        Uptime = string.IsNullOrWhiteSpace(status.Uptime) ? "—" : status.Uptime;
        CanSwitchRegion = status.CanSwitch;
        CanToggleExit = status.CanToggleExit;
        ExitEnabled = status.ExitEnabled;
        ShareButtonText = status.ExitEnabled ? "停止共享出口" : "开启共享出口";

        var active = Regions.FirstOrDefault(region => region.Code == status.ToRegion);
        if (active is not null)
        {
            ActiveRegion = active.Name;
            SelectedRegion = active;
        }

        return true;
    }

    private void SynchronizeGroups(IEnumerable<string>? groups)
    {
        var normalized = (groups ?? [])
            .Where(group => !string.IsNullOrWhiteSpace(group))
            .Select(group => group.Trim())
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .OrderBy(group => group, StringComparer.OrdinalIgnoreCase)
            .ToArray();

        if (!Groups.SequenceEqual(normalized, StringComparer.OrdinalIgnoreCase))
        {
            Groups.Clear();
            foreach (var group in normalized)
            {
                Groups.Add(group);
            }
        }

        HasGroups = Groups.Count > 0;
        if (!Groups.Contains(SelectedFromGroup, StringComparer.OrdinalIgnoreCase))
        {
            SelectedFromGroup = Groups.FirstOrDefault() ?? "";
        }
    }

    private void OnLogReceived(string message)
    {
        Dispatcher.UIThread.Post(() =>
        {
            Logs.Add($"[{DateTime.Now:HH:mm:ss}]  {message}");
            while (Logs.Count > 500)
            {
                Logs.RemoveAt(0);
            }
        });
    }

    private void OnProcessExited(int exitCode)
    {
        Dispatcher.UIThread.Post(async () =>
        {
            IsOwnedProcess = false;
            OnLogReceived($"核心已退出（代码 {exitCode}）");
            await RefreshStatusAsync();
        });
    }

    private static string FriendlyError(Exception exception)
    {
        if (exception is HttpRequestException)
        {
            return "网络请求失败，请检查协调器地址和网络连接。";
        }

        return string.IsNullOrWhiteSpace(exception.Message)
            ? "操作失败，请稍后重试。"
            : exception.Message;
    }

    private static (string Title, string Detail, string Color) ConnectionDescription(
        NodeStatus status,
        bool hasExit)
    {
        if (hasExit)
        {
            return (
                "连接正常",
                status.ExitEnabled
                    ? "正在安全访问，同时为信任网络共享连接"
                    : "安全通道已建立，本机代理可以使用",
                "#22C55E");
        }

        if (status.ExitEnabled)
        {
            return ("共享已开启", "正在为信任网络共享连接", "#22C55E");
        }

        if (status.Role == "client")
        {
            return ("正在等待出口", "暂时没有可用出口，可尝试切换地区", "#F59E0B");
        }

        return ("核心在线", "尚未选择出口，完成连接设置后即可使用", "#3B82F6");
    }

    private static string ShortId(string value)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            return "—";
        }

        return value.Length <= 18 ? value : $"{value[..8]}…{value[^6..]}";
    }

    private string RegionDescription(string regionCode)
    {
        if (string.IsNullOrWhiteSpace(regionCode))
        {
            return "未指定";
        }

        var region = Regions.FirstOrDefault(candidate =>
            string.Equals(candidate.Code, regionCode, StringComparison.OrdinalIgnoreCase));
        return region is null ? regionCode : $"{region.Name}（{region.Code}）";
    }

    private static string RoleDescription(string role) => role switch
    {
        "client" => "访问节点",
        "exit" => "共享出口节点",
        "client+exit" => "访问与共享节点",
        "idle" => "待命节点",
        _ when string.IsNullOrWhiteSpace(role) => "—",
        _ => role,
    };

    private static string TrustDescription(string trustScope) => trustScope switch
    {
        "conservative" => "仅信任圈（推荐）",
        "open" => "开放网络",
        _ when string.IsNullOrWhiteSpace(trustScope) => "—",
        _ => trustScope,
    };

    private static int ParsePort(string address, int fallback)
    {
        var separator = address.LastIndexOf(':');
        return separator >= 0 && int.TryParse(address[(separator + 1)..], out var port)
            ? port
            : fallback;
    }

    partial void OnNoticeChanged(string value) =>
        HasNotice = !string.IsNullOrWhiteSpace(value);

    partial void OnSelectedNetworkModeChanged(NetworkModeOption value)
    {
        IsCreateNetworkMode = value.Value == "create";
        IsJoinNetworkMode = !IsCreateNetworkMode;
        StartButtonText = IsCreateNetworkMode ? "创建并启动网络" : "保存并启动连接";
        if (IsCreateNetworkMode)
        {
            ShareCoordinatorUrl = WhatGateProcessService.GetShareCoordinatorUrl(LocalCoordinatorPort);
        }
    }

    partial void OnLocalCoordinatorPortChanged(int value)
    {
        if (IsCreateNetworkMode && value is >= 1 and <= 65535)
        {
            ShareCoordinatorUrl = WhatGateProcessService.GetShareCoordinatorUrl(value);
        }
    }

    public void Dispose()
    {
        _pollCancellation.Cancel();
        _pollCancellation.Dispose();
        _processService.LogReceived -= OnLogReceived;
        _processService.ProcessExited -= OnProcessExited;
        _processService.Dispose();
        _apiClient.Dispose();
    }
}
