#!/bin/bash
echo "正在为 Windows 编译端口代理管理器..."
GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o portproxy-manager.exe 
if [ $? -eq 0 ]; then
    echo "编译成功！已创建 portproxy-manager.exe（双击后台运行）"
else
    echo "编译失败！"
    exit 1
fi
