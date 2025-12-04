@echo off
echo 正在编译端口代理管理器...
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-H windowsgui" -o portproxy-manager.exe main.go
if %errorlevel% neq 0 (
    echo 编译失败！
    pause
    exit /b %errorlevel%
)
echo 编译成功！已创建 portproxy-manager.exe（双击后台运行）
pause
