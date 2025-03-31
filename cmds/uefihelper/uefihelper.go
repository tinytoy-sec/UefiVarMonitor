package main

import (
	"flag"
	"fmt"

	"github.com/tinytoy-sec/UefiVarMonitor/pkg/log"
	"github.com/tinytoy-sec/UefiVarMonitor/pkg/uefi"
	"github.com/tinytoy-sec/UefiVarMonitor/pkg/uefihelper"
	"github.com/tinytoy-sec/UefiVarMonitor/pkg/visitors"
)

// 配置结构
type config struct {
	ErasePolarity *byte
}

// 解析命令行参数
func parseArguments() (config, []string, error) {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "用法: uefihelper [标志] <文件名> [0个或多个操作]\n")
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\n操作:\n%s", visitors.ListCLI())
	}
	flag.Parse()
	if len(flag.Args()) == 0 || flag.Args()[0] == "help" {
		flag.Usage()
	}
	var cfg config
	return cfg, flag.Args(), nil
}
func main() {
	cfg, args, err := parseArguments()
	if err != nil {
		panic(err)
	}
	if cfg.ErasePolarity != nil {
		if err := uefi.SetErasePolarity(*cfg.ErasePolarity); err != nil {
			panic(fmt.Errorf("出现错误 0x%X: %w", *cfg.ErasePolarity, err))
		}
	}
	if err := uefihelper.Run(args...); err != nil {
		log.Fatalf("%v", err)
	}
}
