package uefi

import (
	"fmt"
)

const (
	// RegionBlockSize 假设区域结构值对应于大小为0x1000的块
	RegionBlockSize = 0x1000
)

// FlashRegionType 表示闪存区域中可能的不同类型
type FlashRegionType int

// IFD 区域类型
// 这也对应于它们在闪存区域部分中的索引
// 参考自github.com/LongSoft/UEFITool, common/descriptor.h
const (
	RegionTypeBIOS FlashRegionType = iota
	RegionTypeME
	RegionTypeGBE
	RegionTypePD
	RegionTypeDevExp1
	RegionTypeBIOS2
	RegionTypeMicrocode
	RegionTypeEC
	RegionTypeDevExp2
	RegionTypeIE
	RegionTypeTGBE1
	RegionTypeTGBE2
	RegionTypeReserved1
	RegionTypeReserved2
	RegionTypePTT

	RegionTypeUnknown FlashRegionType = -1
)

// 闪存区域类型名称映射
var flashRegionTypeNames = map[FlashRegionType]string{
	RegionTypeBIOS:      "BIOS",
	RegionTypeME:        "ME",
	RegionTypeGBE:       "GbE",
	RegionTypePD:        "PD",
	RegionTypeDevExp1:   "DevExp1",
	RegionTypeBIOS2:     "BIOS2",
	RegionTypeMicrocode: "Microcode",
	RegionTypeEC:        "EC",
	RegionTypeDevExp2:   "DevExp2",
	RegionTypeIE:        "IE",
	RegionTypeTGBE1:     "10GbE1",
	RegionTypeTGBE2:     "10GbE2",
	RegionTypeReserved1: "Reserved1",
	RegionTypeReserved2: "Reserved2",
	RegionTypePTT:       "PTT",
	// RegionTypeUnknown 没有字符串名称，我们希望它
	// 回退并打印数字
}

// 返回区域类型的字符串表示
func (rt FlashRegionType) String() string {
	if s, ok := flashRegionTypeNames[rt]; ok {
		return s
	}
	return fmt.Sprintf("未知区域 (%d)", rt)
}

// FlashRegion 保存每种类型区域的基址和限制。每个区域如bios区域
// 都应该指向它
// 待办：确定块大小是从闪存上的某个位置读取还是固定的
// 目前我们假设它们固定为4KiB
type FlashRegion struct {
	Base  uint16 // 第一个4KiB块的索引
	Limit uint16 // 最后一个块的索引
}

// Valid 检查区域是否有效
func (r *FlashRegion) Valid() bool {
	// ODROID bios似乎与所有其他bios不同，并且似乎不正确地报告
	// 无效区域。它们报告限制和基础为0xFFFF而不是限制为0
	return r.Limit > 0 && r.Limit >= r.Base && r.Limit != 0xFFFF && r.Base != 0xFFFF
}

// 返回区域的字符串表示
func (r *FlashRegion) String() string {
	return fmt.Sprintf("[%#x, %#x)", r.Base, r.Limit)
}

// BaseOffset 计算区域在闪存镜像中开始的偏移量
func (r *FlashRegion) BaseOffset() uint32 {
	return uint32(r.Base) * RegionBlockSize
}

// EndOffset 计算区域在闪存镜像中结束的偏移量
func (r *FlashRegion) EndOffset() uint32 {
	return (uint32(r.Limit) + 1) * RegionBlockSize
}

// 区域构造函数映射
var regionConstructors = map[FlashRegionType]func(buf []byte, r *FlashRegion, rt FlashRegionType) (Region, error){
	RegionTypeBIOS:      NewBIOSRegion,
	RegionTypeME:        NewMERegion,
	RegionTypeGBE:       NewRawRegion,
	RegionTypePD:        NewRawRegion,
	RegionTypeDevExp1:   NewRawRegion,
	RegionTypeBIOS2:     NewRawRegion,
	RegionTypeMicrocode: NewRawRegion,
	RegionTypeEC:        NewRawRegion,
	RegionTypeDevExp2:   NewRawRegion,
	RegionTypeIE:        NewRawRegion,
	RegionTypeTGBE1:     NewRawRegion,
	RegionTypeTGBE2:     NewRawRegion,
	RegionTypeReserved1: NewRawRegion,
	RegionTypeReserved2: NewRawRegion,
	RegionTypePTT:       NewRawRegion,
	RegionTypeUnknown:   NewRawRegion,
}

// Region 包含闪存中区域的开始和结束。这可以是BIOS、ME、PDR或GBE区域。
type Region interface {
	Firmware
	Type() FlashRegionType
	FlashRegion() *FlashRegion
	SetFlashRegion(fr *FlashRegion)
}
