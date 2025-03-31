package visitors

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/tinytoy-sec/UefiVarMonitor/pkg/uefi"
)

// 将任何固件节点打印为JSON
type JSON struct {
	// JSON写入到这个writer
	W io.Writer
}

// 包装Visit并执行一些设置和清理任务
func (v *JSON) Run(f uefi.Firmware) error {
	return f.Apply(v)
}

// 将JSON访问者应用于任何固件类型
func (v *JSON) Visit(f uefi.Firmware) error {
	b, err := json.MarshalIndent(f, "", "\t")
	if err != nil {
		return err
	}
	fmt.Fprintln(v.W, string(b))
	return nil
}

func init() {
	RegisterCLI("json", "为整个固件卷生成JSON", 0, func(args []string) (uefi.Visitor, error) {
		return &JSON{
			W: os.Stdout,
		}, nil
	})
}
