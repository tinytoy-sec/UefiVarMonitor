package visitors

import (
	"fmt"
	"sort"

	"github.com/tinytoy-sec/UefiVarMonitor/pkg/uefi"
)

var visitorRegistry = map[string]visitorEntry{}

type visitorEntry struct {
	numArgs       int
	help          string
	createVisitor func([]string) (uefi.Visitor, error)
}

const (
	helpMessage = "用法: uefi-helper 文件 [命令 [参数]]..."
)

// 注册一个函数`createVisitor`，在使用`ParseCLI`解析参数时调用
// 要让访问者可以从命令行访问，应该有一个init函数来注册
// `createVisitor`函数
func RegisterCLI(name string, help string, numArgs int, createVisitor func([]string) (uefi.Visitor, error)) {
	if _, ok := visitorRegistry[name]; ok {
		panic(fmt.Sprintf("两个访问者注册了相同的名称: '%s'", name))
	}
	visitorRegistry[name] = visitorEntry{
		numArgs:       numArgs,
		createVisitor: createVisitor,
		help:          help,
	}
}

// 从给定的CLI参数列表构建访问者列表
func ParseCLI(args []string) ([]uefi.Visitor, error) {
	visitors := []uefi.Visitor{}
	for len(args) > 0 {
		cmd := args[0]
		args = args[1:]
		o, ok := visitorRegistry[cmd]
		if !ok {
			return []uefi.Visitor{}, fmt.Errorf("找不到命令 '%s'\n%s", cmd, helpMessage)
		}
		if o.numArgs > len(args) {
			return []uefi.Visitor{}, fmt.Errorf("命令 '%s' 参数太少，得到 %d，需要 %d。\n概要: %s",
				cmd, len(args), o.numArgs, o.help)
		}
		visitor, err := o.createVisitor(args[:o.numArgs])
		if err != nil {
			return []uefi.Visitor{}, err
		}
		visitors = append(visitors, visitor)
		args = args[o.numArgs:]
	}
	return visitors, nil
}

// 按顺序在固件上应用每个访问者
func ExecuteCLI(f uefi.Firmware, v []uefi.Visitor) error {
	for i := range v {
		if err := v[i].Run(f); err != nil {
			return err
		}
	}
	return nil
}

// 以换行符分隔的字符串形式输出访问者结构中的帮助条目
// 格式为：
//
//	名称: 帮助
func ListCLI() string {
	var s string
	names := []string{}
	for n := range visitorRegistry {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s += fmt.Sprintf("  %-22s: %s\n", n, visitorRegistry[n].help)
	}
	return s
}
