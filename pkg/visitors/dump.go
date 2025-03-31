package visitors

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/tinytoy-sec/UefiVarMonitor/pkg/uefi"
)

// 使用GUID或名称导出固件文件
type Dump struct {
	// 输入
	Predicate func(f uefi.Firmware) bool

	// 输出
	// 文件将写入此writer
	W io.Writer
}

// 只是调用访问者
func (v *Dump) Run(f uefi.Firmware) error {
	return f.Apply(v)
}

// 使用find将文件导出到W
func (v *Dump) Visit(f uefi.Firmware) error {
	// 首先运行"find"生成要导出的列表
	find := Find{
		Predicate: v.Predicate,
	}
	if err := find.Run(f); err != nil {
		return err
	}

	// 必须只有一个匹配项
	if numMatch := len(find.Matches); numMatch > 1 {
		return fmt.Errorf("找到多个匹配项，只允许一个！得到 %v", find.Matches)
	} else if numMatch == 0 {
		return errors.New("未找到匹配项")
	}

	m := find.Matches[0]
	// 注意：导出前可能需要调用assemble，因为缓冲区可能为空
	_, err := v.W.Write(m.Buf())
	return err
}

func init() {
	RegisterCLI("dump", "导出固件文件", 2, func(args []string) (uefi.Visitor, error) {
		pred, err := FindFilePredicate(args[0])
		if err != nil {
			return nil, err
		}

		file, err := os.OpenFile(args[1], os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			return nil, err
		}

		// 找到所有匹配的文件并替换它们的内部PE32
		return &Dump{
			Predicate: pred,
			W:         file,
		}, nil
	})
}
