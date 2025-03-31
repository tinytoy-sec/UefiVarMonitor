package uefi

import (
	"errors"
)

// BIOSPadding 保存固件卷之间的填充
// 这有时可能包含数据，尽管它不应该。我们仍需要保留它。
type BIOSPadding struct {
	buf    []byte
	Offset uint64

	// 元数据
	ExtractPath string
}

// NewBIOSPadding 解析字节序列并返回BIOSPadding对象
func NewBIOSPadding(buf []byte, offset uint64) (*BIOSPadding, error) {
	bp := &BIOSPadding{buf: buf, Offset: offset}
	return bp, nil
}

// Buf 返回缓冲区
func (bp *BIOSPadding) Buf() []byte {
	return bp.buf
}

// SetBuf 设置缓冲区
func (bp *BIOSPadding) SetBuf(buf []byte) {
	bp.buf = buf
}

// Apply 对BIOSPadding应用访问者
func (bp *BIOSPadding) Apply(v Visitor) error {
	return v.Visit(bp)
}

// ApplyChildren 对BIOSPadding的所有直接子节点应用访问者
func (bp *BIOSPadding) ApplyChildren(v Visitor) error {
	return nil
}

// BIOSRegion 表示固件中的BIOS区域
// 它保存所有FV以及填充
type BIOSRegion struct {
	// 保存原始数据
	buf      []byte
	Elements []*TypedFirmware `json:",omitempty"`

	// 提取和恢复的元数据
	ExtractPath string
	Length      uint64
	// 这是指向ifd中布局的FlashRegion结构的指针
	FRegion    *FlashRegion
	RegionType FlashRegionType
}

// Type 返回闪存区域类型
func (br *BIOSRegion) Type() FlashRegionType {
	return RegionTypeBIOS
}

// SetFlashRegion 设置闪存区域
func (br *BIOSRegion) SetFlashRegion(fr *FlashRegion) {
	br.FRegion = fr
}

// FlashRegion 获取闪存区域
func (br *BIOSRegion) FlashRegion() (fr *FlashRegion) {
	return br.FRegion
}

// NewBIOSRegion 解析字节序列并返回Region对象
// 如果传递了有效对象，或者返回错误。它还指向
// ifd中发现的Region结构
func NewBIOSRegion(buf []byte, r *FlashRegion, _ FlashRegionType) (Region, error) {
	br := BIOSRegion{FRegion: r, Length: uint64(len(buf)),
		RegionType: RegionTypeBIOS}
	var absOffset uint64

	// 复制缓冲区
	if ReadOnly {
		br.buf = buf
	} else {
		br.buf = make([]byte, len(buf))
		copy(br.buf, buf)
	}

	for {
		offset := FindFirmwareVolumeOffset(buf)
		if offset < 0 {
			// 未找到固件卷，停止搜索
			// 末尾附近不应该有填充，但以防万一仍然存储它
			if len(buf) != 0 {
				bp, err := NewBIOSPadding(buf, absOffset)
				if err != nil {
					return nil, err
				}
				br.Elements = append(br.Elements, MakeTyped(bp))
			}
			break
		}
		if offset > 0 {
			// 这里有一些填充，以防有数据存储它
			// 我们可以检查并有条件地存储，但这会使事情更复杂
			bp, err := NewBIOSPadding(buf[:offset], absOffset)
			if err != nil {
				return nil, err
			}
			br.Elements = append(br.Elements, MakeTyped(bp))
		}
		absOffset += uint64(offset)                                  // 找到卷的开始相对于bios区域
		fv, err := NewFirmwareVolume(buf[offset:], absOffset, false) // 顶层FV不可调整大小
		if err != nil {
			return nil, err
		}
		if fv.Length == 0 {
			// 避免无限循环
			return nil, errors.New("FV长度为0；无法继续")
		}
		absOffset += fv.Length
		buf = buf[uint64(offset)+fv.Length:]
		br.Elements = append(br.Elements, MakeTyped(fv))
	}
	return &br, nil
}

// Buf 返回缓冲区
// 主要用于与Firmware接口交互的事物
func (br *BIOSRegion) Buf() []byte {
	return br.buf
}

// SetBuf 设置缓冲区
// 主要用于与Firmware接口交互的事物
func (br *BIOSRegion) SetBuf(buf []byte) {
	br.buf = buf
}

// Apply 对BIOSRegion调用访问者
func (br *BIOSRegion) Apply(v Visitor) error {
	return v.Visit(br)
}

// ApplyChildren 对BIOSRegion的每个子节点调用访问者
func (br *BIOSRegion) ApplyChildren(v Visitor) error {
	for _, f := range br.Elements {
		if err := f.Value.Apply(v); err != nil {
			return err
		}
	}
	return nil
}

// FirstFV 查找BIOSRegion中的第一个固件卷
func (br *BIOSRegion) FirstFV() (*FirmwareVolume, error) {
	for _, e := range br.Elements {
		if f, ok := e.Value.(*FirmwareVolume); ok {
			return f, nil
		}
	}
	return nil, errors.New("BIOS区域中没有固件卷")
}
