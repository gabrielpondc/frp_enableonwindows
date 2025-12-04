package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// Config represents application configuration
type Config struct {
	Port              int    `json:"port"`
	FrpcTomlPath      string `json:"frpcTomlPath"`
	FrpcExePath       string `json:"frpcExePath"`
	AutoRegisterToFrp bool   `json:"autoRegisterToFrp"`
	WebUIProxyName    string `json:"webUIProxyName"`
	WebUIRemotePort   int    `json:"webUIRemotePort"`
	Name              string `json:"name"`
}

// Rule represents a portproxy rule
type Rule struct {
	ListenAddress  string `json:"listenAddress"`
	ListenPort     string `json:"listenPort"`
	ConnectAddress string `json:"connectAddress"`
	ConnectPort    string `json:"connectPort"`
}

// FrpProxy represents a proxy configuration in frpc.toml
type FrpProxy struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	LocalIP    string `json:"localIP"`
	LocalPort  string `json:"localPort"`
	RemotePort string `json:"remotePort"`
}

// AddRuleRequest represents the JSON payload for adding a rule
type AddRuleRequest struct {
	ListenPort  string `json:"listenPort"`
	ConnectAddr string `json:"connectAddr"`
	ConnectPort string `json:"connectPort"`
	RemotePort  string `json:"remotePort"`
	Type        string `json:"type"`
	Name        string `json:"name"`
}

var (
	config Config
)

func main() {
	// Load configuration
	if err := loadConfig(); err != nil {
		log.Printf("Warning: Failed to load config.json, using defaults: %v", err)
		config = Config{
			Port:              8080,
			FrpcTomlPath:      "frpc.toml",
			FrpcExePath:       "frpc.exe",
			AutoRegisterToFrp: true,
			WebUIProxyName:    "portproxy-manager-web",
			WebUIRemotePort:   18080,
			Name:              "default",
		}
	}

	// Auto-register web UI to frpc.toml if enabled
	if config.AutoRegisterToFrp {
		if err := registerWebUIToFrpc(); err != nil {
			log.Printf("Warning: Failed to register web UI to frpc.toml: %v", err)
		}
	}

	// Serve static files
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	// API endpoints
	http.HandleFunc("/api/rules", handleGetRules)
	http.HandleFunc("/api/add", handleAddRule)
	http.HandleFunc("/api/default-name", handleGetDefaultName)
	http.HandleFunc("/api/frp-proxies", handleGetFrpProxies)
	http.HandleFunc("/api/frp-proxies/delete", handleDeleteFrpProxy)
	http.HandleFunc("/api/frpc/start", handleStartFrpc)
	http.HandleFunc("/api/frpc/stop", handleStopFrpc)
	http.HandleFunc("/api/frpc/restart", handleRestartFrpc)
	http.HandleFunc("/api/frpc/status", handleFrpcStatus)

	addr := fmt.Sprintf(":%d", config.Port)
	log.Printf("服务器启动在 http://localhost:%d", config.Port)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func loadConfig() error {
	file, err := os.Open("config.json")
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	return decoder.Decode(&config)
}

func registerWebUIToFrpc() error {
	// Check if already registered
	proxies, err := getFrpProxies()
	if err != nil {
		return err
	}

	// The actual proxy name that will be written
	webUIProxyFullName := config.Name + "-" + config.WebUIProxyName

	for _, p := range proxies {
		if p.Name == webUIProxyFullName {
			log.Printf("Web UI 已经注册到 frpc.toml (名称: %s)", webUIProxyFullName)
			return nil
		}
	}

	// Register
	f, err := os.OpenFile(config.FrpcTomlPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	var sb strings.Builder
	sb.WriteString("\n[[proxies]]\n")
	sb.WriteString(fmt.Sprintf("name = \"%s\"\n", webUIProxyFullName))
	sb.WriteString("type = \"tcp\"\n")
	sb.WriteString("localIP = \"127.0.0.1\"\n")
	sb.WriteString(fmt.Sprintf("localPort = %d\n", config.Port))
	sb.WriteString(fmt.Sprintf("remotePort = %d\n", config.WebUIRemotePort))

	if _, err := io.WriteString(f, sb.String()); err != nil {
		return err
	}

	log.Printf("Web UI 已自动注册到 frpc.toml (名称: %s, 远程端口: %d)", webUIProxyFullName, config.WebUIRemotePort)
	return nil
}

func handleGetRules(w http.ResponseWriter, r *http.Request) {
	rules, err := getNetshRules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rules)
}

func handleGetDefaultName(w http.ResponseWriter, r *http.Request) {
	name := config.Name
	if name == "" {
		name = getFirstProxyName()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"name": name})
}

func handleGetFrpProxies(w http.ResponseWriter, r *http.Request) {
	proxies, err := getFrpProxies()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(proxies)
}

func handleDeleteFrpProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := deleteFrpProxy(req.Name); err != nil {
		http.Error(w, "删除 FRP 代理失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Restart frpc
	if err := restartFrpc(); err != nil {
		log.Printf("警告: 重启 frpc 失败: %v", err)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func handleAddRule(w http.ResponseWriter, r *http.Request) {
	var req AddRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 1. Add netsh rule
	if err := addNetshRule(req.ListenPort, req.ConnectAddr, req.ConnectPort); err != nil {
		http.Error(w, "添加 netsh 规则失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Append to frpc.toml
	if err := appendToFrpc(req); err != nil {
		http.Error(w, "更新 frpc.toml 失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 3. Restart frpc
	if err := restartFrpc(); err != nil {
		log.Printf("警告: 重启 frpc 失败: %v", err)
		// Don't fail the request, just log the warning
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func getNetshRules() ([]Rule, error) {
	if runtime.GOOS != "windows" {
		return mockRules(), nil
	}

	cmd := exec.Command("netsh", "interface", "portproxy", "show", "all")
	hideWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseNetshOutput(string(output)), nil
}

func addNetshRule(listenPort, connectAddr, connectPort string) error {
	if runtime.GOOS != "windows" {
		log.Printf("[模拟] netsh interface portproxy add v4tov4 listenaddress=0.0.0.0 listenport=%s connectaddress=%s connectport=%s", listenPort, connectAddr, connectPort)
		return nil
	}

	cmd := exec.Command("netsh", "interface", "portproxy", "add", "v4tov4",
		"listenaddress=0.0.0.0",
		"listenport="+listenPort,
		"connectaddress="+connectAddr,
		"connectport="+connectPort,
	)
	hideWindow(cmd)
	return cmd.Run()
}

func parseNetshOutput(output string) []Rule {
	var rules []Rule
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 4 {
			// Filter out headers
			if fields[0] == "Address" || fields[0] == "---------------" || strings.HasPrefix(fields[0], "Listen") {
				continue
			}
			rules = append(rules, Rule{
				ListenAddress:  fields[0],
				ListenPort:     fields[1],
				ConnectAddress: fields[2],
				ConnectPort:    fields[3],
			})
		}
	}
	return rules
}

func mockRules() []Rule {
	return []Rule{
		{"0.0.0.0", "8080", "192.168.1.10", "80"},
		{"0.0.0.0", "2222", "192.168.1.11", "22"},
	}
}

func getFrpProxies() ([]FrpProxy, error) {
	file, err := os.Open(config.FrpcTomlPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var proxies []FrpProxy
	scanner := bufio.NewScanner(file)

	var current *FrpProxy
	reName := regexp.MustCompile(`^\s*name\s*=\s*"(.*)"`)
	reType := regexp.MustCompile(`^\s*type\s*=\s*"(.*)"`)
	reLocalIP := regexp.MustCompile(`^\s*localIP\s*=\s*"(.*)"`)
	reLocalPort := regexp.MustCompile(`^\s*localPort\s*=\s*(\d+)`)
	reRemotePort := regexp.MustCompile(`^\s*remotePort\s*=\s*(\d+)`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "[[proxies]]" {
			if current != nil {
				proxies = append(proxies, *current)
			}
			current = &FrpProxy{}
			continue
		}

		if current != nil {
			if matches := reName.FindStringSubmatch(line); len(matches) > 1 {
				current.Name = matches[1]
			} else if matches := reType.FindStringSubmatch(line); len(matches) > 1 {
				current.Type = matches[1]
			} else if matches := reLocalIP.FindStringSubmatch(line); len(matches) > 1 {
				current.LocalIP = matches[1]
			} else if matches := reLocalPort.FindStringSubmatch(line); len(matches) > 1 {
				current.LocalPort = matches[1]
			} else if matches := reRemotePort.FindStringSubmatch(line); len(matches) > 1 {
				current.RemotePort = matches[1]
			}
		}
	}

	// Add last proxy
	if current != nil {
		proxies = append(proxies, *current)
	}

	return proxies, nil
}

func getFirstProxyName() string {
	file, err := os.Open(config.FrpcTomlPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inProxies := false
	reName := regexp.MustCompile(`^\s*name\s*=\s*"(.*)"`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "[[proxies]]" {
			inProxies = true
			continue
		}
		if inProxies {
			matches := reName.FindStringSubmatch(line)
			if len(matches) > 1 {
				// Found the first name
				parts := strings.Split(matches[1], "-")
				if len(parts) > 0 {
					return parts[0] // Return the prefix (e.g., "yzwj")
				}
				return matches[1]
			}
		}
	}
	return ""
}

func appendToFrpc(req AddRuleRequest) error {
	f, err := os.OpenFile(config.FrpcTomlPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// New naming convention: [name]-manager-[connectAddr]-[connectPort]
	proxyName := fmt.Sprintf("%s-manager-%s-%s", req.Name, req.ConnectAddr, req.ConnectPort)

	var sb strings.Builder
	sb.WriteString("\n[[proxies]]\n")
	sb.WriteString(fmt.Sprintf("name = \"%s\"\n", proxyName))
	sb.WriteString("type = \"tcp\"\n")
	sb.WriteString("localIP = \"127.0.0.1\"\n")
	sb.WriteString(fmt.Sprintf("localPort = %s\n", req.ListenPort))
	sb.WriteString(fmt.Sprintf("remotePort = %s\n", req.RemotePort))

	if _, err := io.WriteString(f, sb.String()); err != nil {
		return err
	}
	return nil
}

func deleteFrpProxy(proxyName string) error {
	// Read the entire file
	content, err := os.ReadFile(config.FrpcTomlPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	var skipProxy bool
	reName := regexp.MustCompile(`^\s*name\s*=\s*"(.*)"`)

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Check if we're starting a new proxy block
		if trimmed == "[[proxies]]" {
			// Look ahead to check the name
			if i+1 < len(lines) {
				nextLine := lines[i+1]
				if matches := reName.FindStringSubmatch(nextLine); len(matches) > 1 {
					if matches[1] == proxyName {
						// This is the proxy to delete
						skipProxy = true
						continue // Skip the [[proxies]] line
					}
				}
			}
			skipProxy = false
		}

		// If we're in the target proxy block, skip all lines until next [[proxies]]
		if skipProxy {
			// Check if this is the start of a new section
			if trimmed == "[[proxies]]" || (strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
				skipProxy = false
				newLines = append(newLines, line)
			}
			continue
		}

		newLines = append(newLines, line)
	}

	// Write back to file
	return os.WriteFile(config.FrpcTomlPath, []byte(strings.Join(newLines, "\n")), 0644)
}

// ========================================
// FRP Process Management
// ========================================

// getFrpcProcess finds the running frpc process
func getFrpcProcess() (*os.Process, error) {
	if runtime.GOOS != "windows" {
		log.Println("[模拟] 查找 frpc 进程")
		return nil, nil
	}

	// Use tasklist to find frpc.exe
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq frpc.exe", "/FO", "CSV", "/NH")
	hideWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Parse output to check if process exists
	if !strings.Contains(string(output), "frpc.exe") {
		return nil, nil // Process not found
	}

	// Get PID using wmic
	cmd = exec.Command("wmic", "process", "where", "name='frpc.exe'", "get", "ProcessId")
	hideWindow(cmd)
	output, err = cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return nil, nil
	}

	pidStr := strings.TrimSpace(lines[1])
	if pidStr == "" {
		return nil, nil
	}

	var pid int
	fmt.Sscanf(pidStr, "%d", &pid)

	process, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}

	return process, nil
}

// stopFrpc stops the running frpc process
func stopFrpc() error {
	if runtime.GOOS != "windows" {
		log.Println("[模拟] 停止 frpc 进程")
		return nil
	}

	process, err := getFrpcProcess()
	if err != nil {
		return fmt.Errorf("查找进程失败: %v", err)
	}

	if process == nil {
		return nil // Already stopped
	}

	// Kill the process using taskkill for more reliable termination
	cmd := exec.Command("taskkill", "/F", "/IM", "frpc.exe")
	hideWindow(cmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("停止进程失败: %v", err)
	}

	log.Println("frpc 进程已停止")
	return nil
}

// startFrpc starts the frpc process
func startFrpc() error {
	if runtime.GOOS != "windows" {
		log.Println("[模拟] 启动 frpc 进程")
		return nil
	}

	// Check if already running
	process, err := getFrpcProcess()
	if err != nil {
		return fmt.Errorf("检查进程状态失败: %v", err)
	}

	if process != nil {
		return fmt.Errorf("frpc 已经在运行")
	}

	// Start frpc in background
	cmd := exec.Command(config.FrpcExePath, "-c", config.FrpcTomlPath)
	hideWindow(cmd)

	// Redirect output to log files
	logFile, err := os.OpenFile("frpc.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("创建日志文件失败: %v", err)
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("启动 frpc 失败: %v", err)
	}

	// Don't wait for the process
	go func() {
		cmd.Wait()
		logFile.Close()
	}()

	log.Printf("frpc 已启动 (PID: %d, 日志: frpc.log)", cmd.Process.Pid)
	return nil
}

// restartFrpc restarts the frpc process
func restartFrpc() error {
	log.Println("正在重启 frpc...")

	// Stop if running
	if err := stopFrpc(); err != nil {
		log.Printf("警告: 停止 frpc 时出错: %v", err)
	}

	// Wait a moment for the process to fully stop
	if runtime.GOOS == "windows" {
		timeoutCmd := exec.Command("timeout", "/t", "1", "/nobreak")
		hideWindow(timeoutCmd)
		timeoutCmd.Run()
	}

	// Start frpc
	return startFrpc()
}

// getFrpcStatus returns the status of frpc process
func getFrpcStatus() map[string]interface{} {
	status := map[string]interface{}{
		"running": false,
		"pid":     0,
	}

	if runtime.GOOS != "windows" {
		status["running"] = false
		status["message"] = "模拟模式"
		return status
	}

	process, err := getFrpcProcess()
	if err != nil {
		status["error"] = err.Error()
		return status
	}

	if process != nil {
		status["running"] = true
		status["pid"] = process.Pid
	}

	return status
}

// ========================================
// FRP Control API Handlers
// ========================================

func handleStartFrpc(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := startFrpc(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "frpc 已启动"})
}

func handleStopFrpc(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := stopFrpc(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "frpc 已停止"})
}

func handleRestartFrpc(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := restartFrpc(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "frpc 已重启"})
}

func handleFrpcStatus(w http.ResponseWriter, r *http.Request) {
	status := getFrpcStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
