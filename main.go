package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源的连接，生产环境中应该更严格
	},
}

const (
	cmdDir       = "./cmd"          // 命令目录
	cmdTimeout   = 30 * time.Second // 命令执行超时时间
	maxArgLength = 100              // 每个参数最大长度
	maxArgsCount = 10               // 最大参数数量
)

// 添加允许的文件扩展名
var allowedExtensions = map[string]bool{
	".exe": true, // Windows可执行文件
	".bat": true, // Windows批处理文件
	".sh":  true, // Shell脚本
	"":     true, // 无扩展名（Linux/Unix可执行文件）
}

// 验证参数的函数
func validateArgs(args []string) error {
	if len(args) > maxArgsCount {
		return fmt.Errorf("参数数量超过限制 (最大 %d 个)", maxArgsCount)
	}

	for _, arg := range args {
		if len(arg) > maxArgLength {
			return fmt.Errorf("参数长度超过限制 (最大 %d 字符): %s", maxArgLength, arg)
		}
		// 可以添加更多参数验证规则，比如禁止特殊字符等
		if strings.ContainsAny(arg, "<>|&;$") {
			return fmt.Errorf("参数包含非法字符: %s", arg)
		}
	}
	return nil
}

func executeCommand(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "空命令"
	}

	// 验证参数
	if err := validateArgs(parts[1:]); err != nil {
		return fmt.Sprintf("参数验证失败: %v", err)
	}

	// 构建完整的命令路径
	cmdPath := filepath.Join(cmdDir, parts[0])

	// 检查文件扩展名
	ext := filepath.Ext(cmdPath)
	if !allowedExtensions[ext] {
		return fmt.Sprintf("不支持的文件类型: %s", ext)
	}

	// 检查文件是否存在
	if _, err := os.Stat(cmdPath); os.IsNotExist(err) {
		return fmt.Sprintf("命令不存在: %s", parts[0])
	}

	// 检查文件是否在允许的目录下
	absPath, err := filepath.Abs(cmdPath)
	if err != nil {
		return fmt.Sprintf("路径解析错误: %v", err)
	}
	absCmdDir, err := filepath.Abs(cmdDir)
	if err != nil {
		return fmt.Sprintf("命令目录路径解析错误: %v", err)
	}
	if !strings.HasPrefix(absPath, absCmdDir) {
		return "禁止访问cmd目录以外的命令"
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	// 执行命令
	cmd := exec.CommandContext(ctx, cmdPath, parts[1:]...)
	output, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("命令执行超时（超过 %v）", cmdTimeout)
	}

	if err != nil {
		return fmt.Sprintf("执行错误: %v\n", err)
	}

	return string(output)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}
	defer conn.Close()

	for {
		// 读取消息
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("读取错误: %v", err)
			break
		}

		command := string(message)
		log.Printf("收到命令: %s", command)

		// 执行命令并获取结果
		result := executeCommand(command)

		// 发送结果回客户端
		err = conn.WriteMessage(websocket.TextMessage, []byte(result))
		if err != nil {
			log.Printf("写入错误: %v", err)
			break
		}
	}
}

func main() {
	http.HandleFunc("/ws", handleWebSocket)

	fmt.Println("WebSocket服务器启动在 :3000 端口...")
	if err := http.ListenAndServe(":3000", nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
