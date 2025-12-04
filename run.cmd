@echo off
REM 检查 frpc 是否正在运行
tasklist /FI "IMAGENAME eq frpc.exe" | find /I "frpc.exe" >nul
IF %ERRORLEVEL%==0 (
    taskkill /F /IM frpc.exe >nul
)

REM 后台启动 frpc，不弹出 CMD 窗口
start "" /B "C:\Users\70464\Downloads\frp_0.61.1_windows_amd64\frp_0.61.1_windows_amd64\frpc.exe" -c "C:\Users\70464\Downloads\frp_0.61.1_windows_amd64\frp_0.61.1_windows_amd64\frpc.toml"