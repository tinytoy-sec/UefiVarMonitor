package uefi

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// FlashMasterSectionSize 是FlashMaster部分的字节大小
const FlashMasterSectionSize = 12

// RegionPermissions 保存其他区域的读/写权限
type RegionPermissions struct {
	ID    uint16
	Read  uint8
	Write uint8
}

func (r *RegionPermissions) String() string {
	return fmt.Sprintf("RegionPermissions{ID=%v, 读=0x%x, 写=0x%x}",
		r.ID, r.Read, r.Write)
}

// FlashMasterSection 保存所有ID和其他区域的读/写权限
// 这控制BIOS区域是否可以读/写ME等
type FlashMasterSection struct {
	BIOS RegionPermissions
	ME   RegionPermissions
	GBE  RegionPermissions
}

func (m *FlashMasterSection) String() string {
	return fmt.Sprintf("FlashMasterSection{Bios %v, Me %v, Gbe %v}",
		m.BIOS, m.ME, m.GBE)
}

// NewFlashMasterSection 解析字节序列并返回FlashMasterSection对象，
// 如果传递了有效对象，或返回错误
func NewFlashMasterSection(buf []byte) (*FlashMasterSection, error) {
	if len(buf) < FlashMasterSectionSize {
		return nil, fmt.Errorf("闪存主控部分大小太小: 预期%v字节，得到%v",
			FlashMasterSectionSize,
			len(buf),
		)
	}
	var master FlashMasterSection
	reader := bytes.NewReader(buf)
	if err := binary.Read(reader, binary.LittleEndian, &master); err != nil {
		return nil, err
	}
	return &master, nil
}
