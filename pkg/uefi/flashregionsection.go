package uefi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// FlashRegionSectionSize 是区域描述符的大小。它由16个字段组成，每个2x16位大小。
const FlashRegionSectionSize = 64

// FlashRegionSection 保存所有不同闪存区域如PDR、Gbe和Bios区域的元数据
type FlashRegionSection struct {
	_                   uint16
	FlashBlockEraseSize uint16

	// 这在任何地方都没有文档记录，但我只看到过图像有16个FlashRegion条目的槽，
	// FlashMasterSection紧随其后，所以我暂时假设这是最大值。
	FlashRegions [15]FlashRegion
}

// ValidRegions 返回大小非零区域的名称列表
func (f *FlashRegionSection) ValidRegions() []string {
	var regions []string
	for i, r := range f.FlashRegions {
		if r.Valid() {
			regions = append(regions, flashRegionTypeNames[FlashRegionType(i)])
		}
	}
	return regions
}

func (f *FlashRegionSection) String() string {
	return fmt.Sprintf("FlashRegionSection{区域=%v}",
		strings.Join(f.ValidRegions(), ","),
	)
}

// NewFlashRegionSection 从字节切片初始化FlashRegionSection
func NewFlashRegionSection(data []byte) (*FlashRegionSection, error) {
	if len(data) < FlashRegionSectionSize {
		return nil, fmt.Errorf("闪存区域部分大小太小: 预期%v字节，得到%v",
			FlashRegionSectionSize,
			len(data),
		)
	}
	var region FlashRegionSection
	reader := bytes.NewReader(data)
	if err := binary.Read(reader, binary.LittleEndian, &region); err != nil {
		return nil, err
	}
	return &region, nil
}
