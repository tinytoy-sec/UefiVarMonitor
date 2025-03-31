package uefi

// RawRegion 实现了固件镜像中原始字节块的区域
type RawRegion struct {
	// 保存原始数据
	buf []byte
	// 提取和恢复的元数据
	ExtractPath string
	// 指向ifd中布局的FlashRegion结构的指针
	FRegion *FlashRegion
	// IFD中的区域类型
	RegionType FlashRegionType
}

// SetFlashRegion 设置闪存区域
func (rr *RawRegion) SetFlashRegion(fr *FlashRegion) {
	rr.FRegion = fr
}

// FlashRegion 获取闪存区域
func (rr *RawRegion) FlashRegion() (fr *FlashRegion) {
	return rr.FRegion
}

// NewRawRegion 创建一个新区域
func NewRawRegion(buf []byte, r *FlashRegion, rt FlashRegionType) (Region, error) {
	rr := &RawRegion{FRegion: r, RegionType: rt}
	rr.buf = make([]byte, len(buf))
	copy(rr.buf, buf)
	return rr, nil
}

// Type 返回闪存区域类型
func (rr *RawRegion) Type() FlashRegionType {
	return rr.RegionType
}

// Buf 返回缓冲区
// 主要用于与Firmware接口交互
func (rr *RawRegion) Buf() []byte {
	return rr.buf
}

// SetBuf 设置缓冲区
// 主要用于与Firmware接口交互
func (rr *RawRegion) SetBuf(buf []byte) {
	rr.buf = buf
}

// Apply 在RawRegion上调用访问者
func (rr *RawRegion) Apply(v Visitor) error {
	return v.Visit(rr)
}

// ApplyChildren 在RawRegion的每个子节点上调用访问者
func (rr *RawRegion) ApplyChildren(v Visitor) error {
	return nil
}
