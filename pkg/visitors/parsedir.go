package visitors

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/tinytoy-sec/UefiVarMonitor/pkg/uefi"
)

// 创建固件树并从提供的目录读取二进制文件
type ParseDir struct {
	BasePath string
}

// 实际上未实现，因为无法符合接口
func (v *ParseDir) Run(f uefi.Firmware) error {
	return errors.New("ParseDir的Run函数未实现，请勿使用")
}

// 解析目录并创建树
func (v *ParseDir) Parse() (uefi.Firmware, error) {
	// 读取json并构建树
	jsonbuf, err := os.ReadFile(filepath.Join(v.BasePath, "summary.json"))
	if err != nil {
		return nil, err
	}
	f, err := uefi.UnmarshalFirmware(jsonbuf)
	if err != nil {
		return nil, err
	}

	if err = f.Apply(v); err != nil {
		return nil, err
	}
	return f, nil
}

func (v *ParseDir) readBuf(ExtractPath string) ([]byte, error) {
	if ExtractPath != "" {
		return os.ReadFile(filepath.Join(v.BasePath, ExtractPath))
	}
	return nil, nil
}

// 将ParseDir访问者应用于任何固件类型
func (v *ParseDir) Visit(f uefi.Firmware) error {
	var err error
	var fBuf []byte
	switch f := f.(type) {

	case *uefi.FirmwareVolume:
		fBuf, err = v.readBuf(f.ExtractPath)

	case *uefi.File:
		fBuf, err = v.readBuf(f.ExtractPath)

	case *uefi.Section:
		fBuf, err = v.readBuf(f.ExtractPath)

	case *uefi.NVar:
		if f.IsValid() {
			var fValBuf []byte
			fValBuf, err = v.readBuf(f.ExtractPath)
			fBuf = append(make([]byte, f.DataOffset), fValBuf...)
		} else {
			fBuf, err = v.readBuf(f.ExtractPath)
		}

	case *uefi.FlashDescriptor:
		fBuf, err = v.readBuf(f.ExtractPath)

	case *uefi.BIOSRegion:
		fBuf, err = v.readBuf(f.ExtractPath)

	case *uefi.MERegion:
		fBuf, err = v.readBuf(f.ExtractPath)

	case *uefi.RawRegion:
		fBuf, err = v.readBuf(f.ExtractPath)

	case *uefi.BIOSPadding:
		fBuf, err = v.readBuf(f.ExtractPath)
	}

	if err != nil {
		return err
	}
	f.SetBuf(fBuf)

	return f.ApplyChildren(v)
}
