using System.Diagnostics;
using System.Net;
using System.Net.Sockets;
using System.Text.Json;
using WhatGate.Desktop.Models;

namespace WhatGate.Desktop.Services;

public sealed class WhatGateProcessService : IDisposable
{
    private readonly object _sync = new();
    private Process? _process;
    private Process? _coordinatorProcess;
    private string? _temporaryConfigPath;

    public event Action<string>? LogReceived;
    public event Action<int>? ProcessExited;

    public bool IsRunning
    {
        get
        {
            lock (_sync)
            {
                return _process is { HasExited: false };
            }
        }
    }

    public string? CorePath { get; private set; }

    public string? CoordinatorPath { get; private set; }

    public string? ResolveCorePath()
    {
        var executable = OperatingSystem.IsWindows() ? "whatgate.exe" : "whatgate";
        return CorePath = ResolveExecutable("WHATGATE_CORE_PATH", executable);
    }

    public string? ResolveCoordinatorPath()
    {
        var executable = OperatingSystem.IsWindows() ? "coordinator.exe" : "coordinator";
        return CoordinatorPath = ResolveExecutable("WHATGATE_COORDINATOR_PATH", executable);
    }

    public static string GetShareCoordinatorUrl(int port)
    {
        try
        {
            var address = Dns.GetHostEntry(Dns.GetHostName()).AddressList.FirstOrDefault(candidate =>
                candidate.AddressFamily == AddressFamily.InterNetwork
                && !IPAddress.IsLoopback(candidate)
                && !candidate.ToString().StartsWith("169.254.", StringComparison.Ordinal));
            return $"http://{address ?? IPAddress.Loopback}:{port}";
        }
        catch
        {
            return $"http://127.0.0.1:{port}";
        }
    }

    public async Task StartAsync(LaunchOptions options, CancellationToken cancellationToken = default)
    {
        if (IsRunning)
        {
            return;
        }

        var corePath = ResolveCorePath()
            ?? throw new FileNotFoundException(
                "未找到 WhatGate 核心程序。请重新安装，或设置 WHATGATE_CORE_PATH。");

        var coordinatorUrl = options.CoordinatorUrl.Trim();
        if (options.CreateNetwork)
        {
            await StartLocalCoordinatorAsync(options.LocalCoordinatorPort, cancellationToken);
            coordinatorUrl = $"http://127.0.0.1:{options.LocalCoordinatorPort}";
        }

        AppPaths.EnsureDirectories();
        _temporaryConfigPath = Path.Combine(
            AppPaths.RuntimeDirectory,
            $"launch-{Guid.NewGuid():N}.json");

        var config = new Dictionary<string, object>
        {
            ["coordinator"] = coordinatorUrl,
            ["to"] = options.Region,
            ["trust-scope"] = options.TrustScope,
            ["socks"] = $"127.0.0.1:{options.SocksPort}",
            ["web"] = $"127.0.0.1:{options.WebPort}",
            ["identity"] = Path.Combine(AppPaths.DataDirectory, "identity.key"),
            ["coordinator-cache"] = Path.Combine(AppPaths.DataDirectory, "directory.cache"),
            ["member-cert"] = Path.Combine(AppPaths.DataDirectory, "member.json"),
        };

        if (!string.IsNullOrWhiteSpace(options.CoordinatorKey))
        {
            config["coordinator-key"] = options.CoordinatorKey.Trim();
        }

        if (!string.IsNullOrWhiteSpace(options.InvitationCode))
        {
            config["invite"] = options.InvitationCode.Trim();
        }

        if (options.CreateNetwork)
        {
            config["bootstrap-founder"] = true;
            config["exit"] = true;
            config["region"] = options.Region;
        }

        await File.WriteAllTextAsync(
            _temporaryConfigPath,
            JsonSerializer.Serialize(config, new JsonSerializerOptions { WriteIndented = true }),
            cancellationToken);

        if (!OperatingSystem.IsWindows())
        {
            File.SetUnixFileMode(
                _temporaryConfigPath,
                UnixFileMode.UserRead | UnixFileMode.UserWrite);
        }

        var startInfo = new ProcessStartInfo
        {
            FileName = corePath,
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            WorkingDirectory = AppPaths.DataDirectory,
        };
        startInfo.ArgumentList.Add("-config");
        startInfo.ArgumentList.Add(_temporaryConfigPath);

        var process = new Process
        {
            StartInfo = startInfo,
            EnableRaisingEvents = true,
        };

        process.OutputDataReceived += (_, eventArgs) => EmitLog(eventArgs.Data);
        process.ErrorDataReceived += (_, eventArgs) => EmitLog(eventArgs.Data);
        process.Exited += (_, _) =>
        {
            var exitCode = process.ExitCode;
            DeleteTemporaryConfig();
            ProcessExited?.Invoke(exitCode);
        };

        try
        {
            if (!process.Start())
            {
                throw new InvalidOperationException("WhatGate 核心程序未能启动。");
            }
        }
        catch
        {
            DeleteTemporaryConfig();
            process.Dispose();
            if (options.CreateNetwork)
            {
                await StopLocalCoordinatorAsync();
            }
            throw;
        }

        lock (_sync)
        {
            _process = process;
        }

        process.BeginOutputReadLine();
        process.BeginErrorReadLine();
        EmitLog($"核心已启动：{Path.GetFileName(corePath)}");
        _ = DeleteTemporaryConfigAfterReadAsync(process);
    }

    public async Task StopAsync()
    {
        Process? process;
        lock (_sync)
        {
            process = _process;
        }

        if (process is { HasExited: false })
        {
            EmitLog("正在停止核心服务…");
            process.Kill(entireProcessTree: true);
            await process.WaitForExitAsync();
        }

        DeleteTemporaryConfig();
        await StopLocalCoordinatorAsync();
    }

    private async Task StartLocalCoordinatorAsync(int port, CancellationToken cancellationToken)
    {
        lock (_sync)
        {
            if (_coordinatorProcess is { HasExited: false })
            {
                return;
            }
        }

        var coordinatorPath = ResolveCoordinatorPath()
            ?? throw new FileNotFoundException(
                "未找到建网服务。请重新安装客户端，或设置 WHATGATE_COORDINATOR_PATH。");

        AppPaths.EnsureDirectories();
        var startInfo = new ProcessStartInfo
        {
            FileName = coordinatorPath,
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            WorkingDirectory = AppPaths.DataDirectory,
        };
        startInfo.ArgumentList.Add("-addr");
        startInfo.ArgumentList.Add($"0.0.0.0:{port}");
        startInfo.ArgumentList.Add("-invite=");
        startInfo.ArgumentList.Add("-bootstrap-first-member");
        startInfo.ArgumentList.Add("-state");
        startInfo.ArgumentList.Add(Path.Combine(AppPaths.DataDirectory, "coordinator-state.json"));
        startInfo.ArgumentList.Add("-relay-listen=");

        var process = new Process
        {
            StartInfo = startInfo,
            EnableRaisingEvents = true,
        };
        process.OutputDataReceived += (_, eventArgs) => EmitLog(
            string.IsNullOrWhiteSpace(eventArgs.Data) ? null : $"建网服务 · {eventArgs.Data}");
        process.ErrorDataReceived += (_, eventArgs) => EmitLog(
            string.IsNullOrWhiteSpace(eventArgs.Data) ? null : $"建网服务 · {eventArgs.Data}");
        process.Exited += (_, _) => EmitLog($"建网服务已退出（代码 {process.ExitCode}）");

        if (!process.Start())
        {
            process.Dispose();
            throw new InvalidOperationException("本地建网服务未能启动。");
        }
        lock (_sync)
        {
            _coordinatorProcess = process;
        }
        process.BeginOutputReadLine();
        process.BeginErrorReadLine();
        EmitLog($"本地建网服务已启动 · 端口 {port}");

        using var http = new HttpClient { Timeout = TimeSpan.FromMilliseconds(500) };
        for (var attempt = 0; attempt < 30; attempt++)
        {
            cancellationToken.ThrowIfCancellationRequested();
            if (process.HasExited)
            {
                break;
            }
            try
            {
                using var response = await http.GetAsync(
                    $"http://127.0.0.1:{port}/directory",
                    cancellationToken);
                if (response.IsSuccessStatusCode)
                {
                    return;
                }
            }
            catch (HttpRequestException)
            {
                // The process is still starting.
            }
            catch (TaskCanceledException) when (!cancellationToken.IsCancellationRequested)
            {
                // Retry until the bounded startup window expires.
            }
            await Task.Delay(100, cancellationToken);
        }

        await StopLocalCoordinatorAsync();
        throw new InvalidOperationException(
            $"本地建网服务无法监听端口 {port}。请确认端口未被其他程序占用。");
    }

    private async Task StopLocalCoordinatorAsync()
    {
        Process? process;
        lock (_sync)
        {
            process = _coordinatorProcess;
            _coordinatorProcess = null;
        }
        if (process is null)
        {
            return;
        }
        if (!process.HasExited)
        {
            EmitLog("正在停止本地建网服务…");
            process.Kill(entireProcessTree: true);
            await process.WaitForExitAsync();
        }
        process.Dispose();
    }

    private async Task DeleteTemporaryConfigAfterReadAsync(Process process)
    {
        await Task.Delay(TimeSpan.FromSeconds(3));
        if (!process.HasExited)
        {
            DeleteTemporaryConfig();
        }
    }

    private void DeleteTemporaryConfig()
    {
        var path = Interlocked.Exchange(ref _temporaryConfigPath, null);
        if (path is null)
        {
            return;
        }

        try
        {
            File.Delete(path);
        }
        catch
        {
            // Best-effort cleanup. The runtime directory contains no persistent
            // settings and is retried on the next launch.
        }
    }

    private void EmitLog(string? message)
    {
        if (!string.IsNullOrWhiteSpace(message))
        {
            LogReceived?.Invoke(message);
        }
    }

    private static bool IsExecutable(string? path) =>
        !string.IsNullOrWhiteSpace(path) && File.Exists(path);

    private static string? ResolveExecutable(string environmentVariable, string executable)
    {
        var overridePath = Environment.GetEnvironmentVariable(environmentVariable);
        if (IsExecutable(overridePath))
        {
            return Path.GetFullPath(overridePath!);
        }

        var candidates = new List<string>
        {
            Path.Combine(AppContext.BaseDirectory, "core", executable),
            Path.Combine(AppContext.BaseDirectory, executable),
            Path.Combine(Environment.CurrentDirectory, "bin", executable),
        };
        var directory = new DirectoryInfo(AppContext.BaseDirectory);
        for (var i = 0; i < 9 && directory is not null; i++, directory = directory.Parent)
        {
            candidates.Add(Path.Combine(directory.FullName, "bin", executable));
        }
        return candidates
            .Select(Path.GetFullPath)
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .FirstOrDefault(IsExecutable);
    }

    public void Dispose()
    {
        Process? process;
        lock (_sync)
        {
            process = _process;
            _process = null;
        }

        if (process is { HasExited: false })
        {
            process.Kill(entireProcessTree: true);
        }

        process?.Dispose();

        Process? coordinator;
        lock (_sync)
        {
            coordinator = _coordinatorProcess;
            _coordinatorProcess = null;
        }
        if (coordinator is { HasExited: false })
        {
            coordinator.Kill(entireProcessTree: true);
        }
        coordinator?.Dispose();
        DeleteTemporaryConfig();
    }
}
