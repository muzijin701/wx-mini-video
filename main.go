package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/MuZiJin701/wx-mini-video/internal/app"
	"github.com/MuZiJin701/wx-mini-video/internal/config"
	"github.com/MuZiJin701/wx-mini-video/internal/tui"
	"github.com/MuZiJin701/wx-mini-video/internal/certificate"
	"github.com/MuZiJin701/wx-mini-video/internal/system"
)

var AppVer = "v0.2.1"
var Mode = "debug"

func main() {
	if handled, err := certificate.RunHelper(os.Args[1:]); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			system.ShowStartupError(err)
			os.Exit(1)
		}
		return
	}
	relaunched, err := system.PrepareConsole()
	if err != nil {
		system.ShowStartupError(err)
		os.Exit(1)
	}
	if relaunched {
		return
	}
	if wantsDebug(os.Args[1:]) {
		// 参数在 UAC/终端重启时会被转发；显式设置环境变量供嗅探插件读取。
		_ = os.Setenv("WX_MINI_VIDEO_DEBUG", "1")
	}
	cfg := config.New(AppVer, Mode)
	if err := cfg.LoadConfig(); err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		os.Exit(1)
	}
	runtime := app.NewRuntime(cfg)
	defer func() {
		if err := runtime.Stop(); err != nil {
			system.ShowStartupError(fmt.Errorf("恢复系统代理或关闭代理失败: %w", err))
		}
	}()
	if err := tui.Run(runtime); err != nil {
		system.ShowStartupError(errors.Join(err, runtime.Stop()))
		os.Exit(1)
	}
}

// wantsDebug 识别 --debug / -debug；该参数会随 UAC 与 Terminal 重启转发，
// 避免依赖环境变量在提权后的继承行为。
func wantsDebug(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--debug", "-debug":
			return true
		}
	}
	return false
}
