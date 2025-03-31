package uefi

import (
	"fmt"
)

const (
	// FlashParamsSize 是FlashParams结构的大小
	FlashParamsSize = 4
)

// FlashFrequency 是用于频率字段的类型
type FlashFrequency uint

// 闪存频率常量
const (
	Freq20MHz      FlashFrequency = 0
	Freq33MHz      FlashFrequency = 1
	Freq48MHz      FlashFrequency = 2
	Freq50MHz30MHz FlashFrequency = 4
	Freq17MHz      FlashFrequency = 6
)

// FlashFrequencyStringMap 将频率常量映射到字符串
var FlashFrequencyStringMap = map[FlashFrequency]string{
	Freq20MHz:      "20MHz",
	Freq33MHz:      "33MHz",
	Freq48MHz:      "48MHz",
	Freq50MHz30MHz: "50Mhz30MHz",
	Freq17MHz:      "17MHz",
}

// FlashParams 是一个4字节对象，保存闪存参数信息
type FlashParams [4]byte

// FirstChipDensity 返回第一个芯片的大小
func (p *FlashParams) FirstChipDensity() uint {
	return uint(p[0] & 0x0f)
}

// SecondChipDensity 返回第二个芯片的大小
func (p *FlashParams) SecondChipDensity() uint {
	return uint((p[0] >> 4) & 0x0f)
}

// ReadClockFrequency 返回从闪存读取时的芯片频率
func (p *FlashParams) ReadClockFrequency() FlashFrequency {
	return FlashFrequency((p[2] >> 1) & 0x07)
}

// FastReadEnabled 返回是否启用FastRead
func (p *FlashParams) FastReadEnabled() uint {
	return uint((p[2] >> 4) & 0x01)
}

// FastReadFrequency 返回FastRead下的频率
func (p *FlashParams) FastReadFrequency() FlashFrequency {
	return FlashFrequency((p[2] >> 5) & 0x07)
}

// FlashWriteFrequency 返回写入的芯片频率
func (p *FlashParams) FlashWriteFrequency() FlashFrequency {
	return FlashFrequency(p[3] & 0x07)
}

// FlashReadStatusFrequency 返回读取闪存状态时的芯片频率
func (p *FlashParams) FlashReadStatusFrequency() FlashFrequency {
	return FlashFrequency((p[3] >> 3) & 0x07)
}

// DualOutputFastReadSupported 返回是否支持双输出快速读取
func (p *FlashParams) DualOutputFastReadSupported() uint {
	return uint(p[3] >> 7)
}

func (p *FlashParams) String() string {
	return "FlashParams{...}"
}

// NewFlashParams 从字节切片初始化FlashParam结构
func NewFlashParams(buf []byte) (*FlashParams, error) {
	if len(buf) != FlashParamsSize {
		return nil, fmt.Errorf("无效的镜像大小: 预期%v字节，得到%v",
			FlashParamsSize,
			len(buf),
		)
	}
	var p FlashParams
	copy(p[:], buf[0:FlashParamsSize])
	return &p, nil
}
