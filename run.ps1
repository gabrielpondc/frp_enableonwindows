# -----------------------------
# frpc 自动启动脚本 (静默后台)
# -----------------------------

# frpc 可执行文件路径
$frpcExe = "C:\Users\70464\Downloads\frp_0.61.1_windows_amd64\frp_0.61.1_windows_amd64\frpc.exe"

# frpc 配置文件路径
$frpcIni = "C:\Users\70464\Downloads\frp_0.61.1_windows_amd64\frp_0.61.1_windows_amd64\frpc.toml"

# -----------------------------
# 查询 frpc 进程
# -----------------------------
$process = Get-Process -Name "frpc" -ErrorAction SilentlyContinue

if ($process) {
    Write-Host "检测到 frpc 正在运行，正在终止..."
    $process | Stop-Process -Force
} else {
    Write-Host "frpc 未运行，准备启动..."
}

# -----------------------------
# 启动 frpc 静默运行
# -----------------------------
Write-Host "正在后台启动 frpc..."

Start-Process -FilePath $frpcExe `
              -ArgumentList "-c `"$frpcIni`"" `
              -WindowStyle Hidden `
              -RedirectStandardOutput "$env:TEMP\frpc.log" `
              -RedirectStandardError "$env:TEMP\frpc.err"

Write-Host "frpc 已启动 (日志: $env:TEMP\frpc.log / $env:TEMP\frpc.err)"