package uefi

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	// FlashDescriptorMapMaxBase 是闪存描述符区域的最大基址
	FlashDescriptorMapMaxBase = 0xe0

	// FlashDescriptorMapSize 是FlashDescriptorMap的字节大小
	FlashDescriptorMapSize = 16
)

// FlashDescriptorMap 表示Intel闪存描述符。此对象提供访问各种描述符字段的方法。
type FlashDescriptorMap struct {
	// FLMAP0
	ComponentBase      uint8
	NumberOfFlashChips uint8
	RegionBase         uint8
	NumberOfRegions    uint8
	// FLMAP1
	MasterBase        uint8
	NumberOfMasters   uint8
	PchStrapsBase     uint8
	NumberOfPchStraps uint8
	// FLMAP2
	ProcStrapsBase          uint8
	NumberOfProcStraps      uint8
	IccTableBase            uint8
	NumberOfIccTableEntries uint8
	// FLMAP3
	DmiTableBase            uint8
	NumberOfDmiTableEntries uint8
	Reserved0               uint8
	Reserved1               uint8
}

// NewFlashDescriptorMap 从字节切片初始化FlashDescriptor
func NewFlashDescriptorMap(buf []byte) (*FlashDescriptorMap, error) {
	r := bytes.NewReader(buf)
	var descriptor FlashDescriptorMap
	if err := binary.Read(r, binary.LittleEndian, &descriptor); err != nil {
		return nil, err
	}
	return &descriptor, nil
}

func (d *FlashDescriptorMap) String() string {
	return fmt.Sprintf("FlashDescriptorMap{区域数量=%v, 闪存芯片数量=%v, 主控数量=%v, PCH带数=%v, 处理器带数=%v, ICC表条目数=%v, DMI表条目数=%v}",
		d.NumberOfRegions,
		d.NumberOfFlashChips,
		d.NumberOfMasters,
		d.NumberOfPchStraps,
		d.NumberOfProcStraps,
		d.NumberOfIccTableEntries,
		d.NumberOfDmiTableEntries,
	)
}
