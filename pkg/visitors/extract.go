package visitors

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinytoy-sec/UefiVarMonitor/pkg/uefi"
)

var (
	force  = flag.Bool("force", false, "强制提取到非空目录")
	remove = flag.Bool("remove", false, "提取前删除现有目录")
)

// 将任何固件节点提取到DirPath
type Extract struct {
	BasePath string
	DirPath  string
	Index    *uint64
}

// 将二进制文件简单地转储到指定目录和文件名
// 如果目录不存在则创建，并将缓冲区转储到其中
// 返回二进制文件的文件路径，如果存在错误则返回错误
// 这是作为其他Extract函数的辅助函数
func (v *Extract) extractBinary(buf []byte, filename string) (string, error) {
	// 如果目录不存在则创建
	dirPath := filepath.Join(v.BasePath, v.DirPath)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", err
	}

	// 转储二进制文件
	fp := filepath.Join(dirPath, filename)
	if err := os.WriteFile(fp, buf, 0666); err != nil {
		// 确保返回""，因为我们不希望无效路径被序列化出去
		return "", err
	}
	// 只返回相对于树根的相对路径
	return filepath.Join(v.DirPath, filename), nil
}

// 包装Visit并执行一些设置和清理任务
func (v *Extract) Run(f uefi.Firmware) error {
	// 如果目录已存在，可选择删除
	if *remove {
		if err := os.RemoveAll(v.BasePath); err != nil {
			return err
		}
	}

	if !*force {
		// 检查目录是否不存在或为空
		files, err := os.ReadDir(v.BasePath)
		if err == nil {
			if len(files) != 0 {
				return errors.New("现有目录非空，使用--force覆盖")
			}
		} else if !os.IsNotExist(err) {
			// 错误不是EEXIST，我们不知道出了什么问题
			return err
		}
	}

	// 如果目录不存在，则创建
	if err := os.MkdirAll(v.BasePath, 0755); err != nil {
		return err
	}

	// 重置索引
	*v.Index = 0
	if err := f.Apply(v); err != nil {
		return err
	}

	// 输出摘要json
	json, err := uefi.MarshalFirmware(f)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(v.BasePath, "summary.json"), json, 0666)
}

// 将Extract访问者应用于任何固件类型
func (v *Extract) Visit(f uefi.Firmware) error {
	// 在修改前必须克隆访问者；否则，会修改兄弟节点的值
	v2 := *v

	var err error
	switch f := f.(type) {

	case *uefi.FirmwareVolume:
		v2.DirPath = filepath.Join(v.DirPath, fmt.Sprintf("%#x", f.FVOffset))
		if len(f.Files) == 0 {
			f.ExtractPath, err = v2.extractBinary(f.Buf(), "fv.bin")
		} else {
			f.ExtractPath, err = v2.extractBinary(f.Buf()[:f.DataOffset], "fvh.bin")
		}

	case *uefi.File:
		// 对于文件，我们使用GUID作为文件夹名称
		v2.DirPath = filepath.Join(v.DirPath, f.Header.GUID.String())
		// 使ID唯一的简单方法
		v2.DirPath = filepath.Join(v2.DirPath, fmt.Sprint(*v.Index))
		*v.Index++
		if len(f.Sections) == 0 && f.NVarStore == nil {
			f.ExtractPath, err = v2.extractBinary(f.Buf(), fmt.Sprintf("%v.ffs", f.Header.GUID))
		}

	case *uefi.Section:
		// 对于部分，我们使用文件顺序作为文件夹名称
		v2.DirPath = filepath.Join(v.DirPath, fmt.Sprint(f.FileOrder))
		if len(f.Encapsulated) == 0 {
			f.ExtractPath, err = v2.extractBinary(f.Buf(), fmt.Sprintf("%v.sec", f.FileOrder))
		}

	case *uefi.NVar:
		// 对于NVar，我们使用GUID作为文件夹名称，Name作为文件名，并添加偏移量使链接唯一
		v2.DirPath = filepath.Join(v.DirPath, f.GUID.String())
		if f.IsValid() {
			if f.NVarStore == nil {
				if f.Type == uefi.LinkNVarEntry {
					f.ExtractPath, err = v2.extractBinary(f.Buf()[f.DataOffset:], fmt.Sprintf("%v-%#x.bin", f.Name, f.Offset))
				} else {
					f.ExtractPath, err = v2.extractBinary(f.Buf()[f.DataOffset:], fmt.Sprintf("%v.bin", f.Name))
				}
			}
		} else {
			f.ExtractPath, err = v2.extractBinary(f.Buf(), fmt.Sprintf("%#x.nvar", f.Offset))
		}

	case *uefi.FlashDescriptor:
		v2.DirPath = filepath.Join(v.DirPath, "ifd")
		f.ExtractPath, err = v2.extractBinary(f.Buf(), "flashdescriptor.bin")

	case *uefi.BIOSRegion:
		v2.DirPath = filepath.Join(v.DirPath, "bios")
		if len(f.Elements) == 0 {
			f.ExtractPath, err = v2.extractBinary(f.Buf(), "biosregion.bin")
		}

	case *uefi.MERegion:
		v2.DirPath = filepath.Join(v.DirPath, "me")
		f.ExtractPath, err = v2.extractBinary(f.Buf(), "meregion.bin")

	case *uefi.RawRegion:
		v2.DirPath = filepath.Join(v.DirPath, f.Type().String())
		f.ExtractPath, err = v2.extractBinary(f.Buf(), fmt.Sprintf("%#x.bin", f.FlashRegion().BaseOffset()))

	case *uefi.BIOSPadding:
		v2.DirPath = filepath.Join(v.DirPath, fmt.Sprintf("biospad_%#x", f.Offset))
		f.ExtractPath, err = v2.extractBinary(f.Buf(), "pad.bin")
	}
	if err != nil {
		return err
	}

	return f.ApplyChildren(&v2)
}

func init() {
	var fileIndex uint64
	RegisterCLI("extract", "将文件提取到目录", 1, func(args []string) (uefi.Visitor, error) {
		return &Extract{
			BasePath: args[0],
			DirPath:  ".",
			Index:    &fileIndex,
		}, nil
	})
}
