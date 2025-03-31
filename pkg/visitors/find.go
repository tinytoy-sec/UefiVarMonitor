package visitors

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/tinytoy-sec/UefiVarMonitor/pkg/guid"
	"github.com/tinytoy-sec/UefiVarMonitor/pkg/log"
	"github.com/tinytoy-sec/UefiVarMonitor/pkg/uefi"
)

// 用于筛选Find访问者匹配项的谓词
type FindPredicate = func(f uefi.Firmware) bool

// 根据名称或GUID查找固件文件
type Find struct {
	// 输入
	// 只有当此函数返回true时，文件才会出现在`Matches`切片中
	Predicate FindPredicate

	// 输出
	Matches []uefi.Firmware

	// JSON写入此writer
	W io.Writer

	// 私有
	currentFile *uefi.File
}

// 包装Visit并执行一些设置和清理任务
func (v *Find) Run(f uefi.Firmware) error {
	if err := f.Apply(v); err != nil {
		return err
	}
	if v.W != nil {
		b, err := json.MarshalIndent(v.Matches, "", "\t")
		if err != nil {
			log.Fatalf("%v", err)
		}
		fmt.Fprintln(v.W, string(b))
	}
	return nil
}

// 将Find访问者应用于任何固件类型
func (v *Find) Visit(f uefi.Firmware) error {
	switch f := f.(type) {

	case *uefi.File:
		// 克隆访问者，使`currentFile`仅传递给后代
		v2 := &Find{
			Predicate:   v.Predicate,
			currentFile: f,
		}

		if v.Predicate(f) {
			v.Matches = append(v.Matches, f)
			// 不与直接后代匹配
			v2.currentFile = nil
		}

		err := f.ApplyChildren(v2)
		v.Matches = append(v.Matches, v2.Matches...) // 合并在一起
		return err

	case *uefi.Section:
		if v.currentFile != nil && v.Predicate(f) {
			v.Matches = append(v.Matches, v.currentFile)
			v.currentFile = nil // 如果有重名，不要与兄弟节点重复匹配
		}
		return f.ApplyChildren(v)

	case *uefi.NVar:
		// 不查找嵌入在NVar中的NVar（TODO：为此添加参数？）
		if v.Predicate(f) {
			v.Matches = append(v.Matches, f)
		}
		return nil

	default:
		if v.Predicate(f) {
			v.Matches = append(v.Matches, f)
		}
		return f.ApplyChildren(v)
	}
}

// 仅搜索文件GUID的通用谓词
func FindFileGUIDPredicate(r guid.GUID) FindPredicate {
	return func(f uefi.Firmware) bool {
		if f, ok := f.(*uefi.File); ok {
			return f.Header.GUID == r
		}
		return false
	}
}

// 仅搜索文件类型的通用谓词
func FindFileTypePredicate(t uefi.FVFileType) FindPredicate {
	return func(f uefi.Firmware) bool {
		if f, ok := f.(*uefi.File); ok {
			return f.Header.Type == t
		}
		return false
	}
}

// 仅搜索文件和UI部分的通用谓词
func FindFilePredicate(r string) (func(f uefi.Firmware) bool, error) {
	ciRE, err := regexp.Compile("^(?i)(" + r + ")$")
	if err != nil {
		return nil, err
	}
	return func(f uefi.Firmware) bool {
		switch f := f.(type) {
		case *uefi.File:
			return ciRE.MatchString(f.Header.GUID.String())
		case *uefi.Section:
			return ciRE.MatchString(f.Name)
		}
		return false
	}, nil
}

// 搜索FV、文件和UI部分的通用谓词
func FindFileFVPredicate(r string) (func(f uefi.Firmware) bool, error) {
	ciRE, err := regexp.Compile("^(?i)" + r + "$")
	if err != nil {
		return nil, err
	}
	return func(f uefi.Firmware) bool {
		switch f := f.(type) {
		case *uefi.FirmwareVolume:
			return ciRE.MatchString(f.FVName.String())
		case *uefi.File:
			return ciRE.MatchString(f.Header.GUID.String())
		case *uefi.Section:
			return ciRE.MatchString(f.Name)
		}
		return false
	}, nil
}

// 对现有谓词取逻辑非的通用谓词
func FindNotPredicate(predicate FindPredicate) FindPredicate {
	return func(f uefi.Firmware) bool {
		return !predicate(f)
	}
}

// 对两个现有谓词取逻辑与的通用谓词
func FindAndPredicate(predicate1 FindPredicate, predicate2 FindPredicate) FindPredicate {
	return func(f uefi.Firmware) bool {
		return predicate1(f) && predicate2(f)
	}
}

// 使用提供的谓词进行查找，如果有多个结果则报错
func FindExactlyOne(f uefi.Firmware, pred func(f uefi.Firmware) bool) (uefi.Firmware, error) {
	find := &Find{
		Predicate: pred,
	}
	if err := find.Run(f); err != nil {
		return nil, err
	}
	// 应该只有一个匹配项，应该只有一个Dxe核心
	if mlen := len(find.Matches); mlen != 1 {
		return nil, fmt.Errorf("期望恰好一个匹配项，得到 %v，匹配项为：%v", mlen, find.Matches)
	}
	return find.Matches[0], nil
}

// 查找包含文件的FV
func FindEnclosingFV(f uefi.Firmware, file *uefi.File) (*uefi.FirmwareVolume, error) {
	pred := func(f uefi.Firmware) bool {
		switch f := f.(type) {
		case *uefi.FirmwareVolume:
			for _, v := range f.Files {
				if v == file {
					return true
				}
			}
		}
		return false
	}
	dxeFV, err := FindExactlyOne(f, pred)
	if err != nil {
		return nil, fmt.Errorf("无法找到DXE FV，得到：%v", err)
	}
	// 结果必须是FV
	fv, ok := dxeFV.(*uefi.FirmwareVolume)
	if !ok {
		return nil, fmt.Errorf("结果不是固件卷！类型为 %T", dxeFV)
	}

	return fv, nil
}

// 快速检索包含DxeCore的固件卷的辅助函数
func FindDXEFV(f uefi.Firmware) (*uefi.FirmwareVolume, error) {
	// 我们通过DxeCore的存在来识别Dxe固件卷
	// 如果有多个dxe卷，这将导致问题
	pred := FindFileTypePredicate(uefi.FVFileTypeDXECore)
	dxeCore, err := FindExactlyOne(f, pred)
	if err != nil {
		return nil, fmt.Errorf("无法找到DXE核心，得到：%v", err)
	}

	// 结果必须是File
	file, ok := dxeCore.(*uefi.File)
	if !ok {
		return nil, fmt.Errorf("结果不是文件！类型为 %T", file)
	}
	return FindEnclosingFV(f, file)
}

func init() {
	RegisterCLI("find", "通过GUID或名称查找文件", 1, func(args []string) (uefi.Visitor, error) {
		pred, err := FindFilePredicate(args[0])
		if err != nil {
			return nil, err
		}
		return &Find{
			Predicate: pred,
			W:         os.Stdout,
		}, nil
	})
}
