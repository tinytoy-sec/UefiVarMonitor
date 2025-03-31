package uefi

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
)

// FlashSignature 是Flash镜像预期以其开始的字节序列
var (
	FlashSignature = []byte{0x5a, 0xa5, 0xf0, 0x0f}
	ErrTooShort    = errors.New("太小，无法成为固件")
)

const (
	// FlashDescriptorLength 表示描述符区域的大小
	FlashDescriptorLength = 0x1000
)

// FlashDescriptor 是表示Intel闪存描述符的主要结构
type FlashDescriptor struct {
	// 保存原始缓冲区
	buf                []byte
	DescriptorMapStart uint
	RegionStart        uint
	MasterStart        uint
	DescriptorMap      *FlashDescriptorMap
	Region             *FlashRegionSection
	Master             *FlashMasterSection

	// 提取和恢复的元数据
	ExtractPath string
}

// FindSignature 搜索Intel闪存签名
func FindSignature(buf []byte) (int, error) {
	if len(buf) < 20 {
		return -1, ErrTooShort
	}
	if bytes.Equal(buf[16:16+len(FlashSignature)], FlashSignature) {
		// 16 + 4，因为描述符在签名之后开始
		return 20, nil
	}
	if len(buf) >= len(FlashSignature) && bytes.Equal(buf[:len(FlashSignature)], FlashSignature) {
		// + 4，因为描述符在签名之后开始
		return len(FlashSignature), nil
	}

	firstBytesCnt := 20
	if len(buf) < firstBytesCnt {
		firstBytesCnt = len(buf)
	}
	return -1, fmt.Errorf("未找到闪存签名: 前%d个字节是:\n%s:%w",
		firstBytesCnt, hex.Dump(buf[:firstBytesCnt]), os.ErrNotExist)
}

// Buf 返回缓冲区
// 主要用于与Firmware接口交互的东西
func (fd *FlashDescriptor) Buf() []byte {
	return fd.buf
}

// SetBuf 设置缓冲区
// 主要用于与Firmware接口交互的东西
func (fd *FlashDescriptor) SetBuf(buf []byte) {
	fd.buf = buf
}

// Apply 在FlashDescriptor上调用访问者
func (fd *FlashDescriptor) Apply(v Visitor) error {
	return v.Visit(fd)
}

// ApplyChildren 在FlashDescriptor的每个子节点上调用访问者
func (fd *FlashDescriptor) ApplyChildren(v Visitor) error {
	return nil
}

// ParseFlashDescriptor 从缓冲区解析ifd
func (fd *FlashDescriptor) ParseFlashDescriptor() error {
	if buflen := len(fd.buf); buflen != FlashDescriptorLength {
		return fmt.Errorf("闪存描述符长度不是%#x，是%#x", FlashDescriptorLength, buflen)
	}

	descriptorMapStart, err := FindSignature(fd.buf)
	if err != nil {
		return err
	}
	fd.DescriptorMapStart = uint(descriptorMapStart)

	// 描述符映射
	desc, err := NewFlashDescriptorMap(fd.buf[fd.DescriptorMapStart:])
	if err != nil {
		return err
	}
	fd.DescriptorMap = desc

	// 区域
	fd.RegionStart = uint(fd.DescriptorMap.RegionBase) * 0x10
	regionEnd := fd.RegionStart + uint(FlashRegionSectionSize)
	if buflen := uint(len(fd.buf)); fd.RegionStart >= buflen || regionEnd >= buflen {
		return fmt.Errorf("闪存描述符区域超出范围: 范围[%#x:%#x]，缓冲区长度%#x", fd.RegionStart, regionEnd, buflen)
	}
	region, err := NewFlashRegionSection(fd.buf[fd.RegionStart:regionEnd])
	if err != nil {
		return err
	}
	fd.Region = region

	// Master
	fd.MasterStart = uint(fd.DescriptorMap.MasterBase) * 0x10
	master, err := NewFlashMasterSection(fd.buf[fd.MasterStart : fd.MasterStart+uint(FlashMasterSectionSize)])
	if err != nil {
		return err
	}
	fd.Master = master

	return nil
}

// FlashImage is the main structure that represents an Intel Flash image. It
// implements the Firmware interface.
type FlashImage struct {
	// Holds the raw buffer
	buf []byte
	// Holds the Flash Descriptor
	IFD FlashDescriptor
	// Actual regions
	Regions []*TypedFirmware `json:",omitempty"`

	// Metadata for extraction and recovery
	ExtractPath string
	FlashSize   uint64
}

// Buf returns the buffer.
// Used mostly for things interacting with the Firmware interface.
func (f *FlashImage) Buf() []byte {
	return f.buf
}

// SetBuf sets the buffer.
// Used mostly for things interacting with the Firmware interface.
func (f *FlashImage) SetBuf(buf []byte) {
	f.buf = buf
}

// Apply calls the visitor on the FlashImage.
func (f *FlashImage) Apply(v Visitor) error {
	return v.Visit(f)
}

// ApplyChildren calls the visitor on each child node of FlashImage.
func (f *FlashImage) ApplyChildren(v Visitor) error {
	if err := f.IFD.Apply(v); err != nil {
		return err
	}
	for _, r := range f.Regions {
		if err := r.Value.Apply(v); err != nil {
			return err
		}
	}
	return nil
}

// IsPCH returns whether the flash image has the more recent PCH format, or not.
// PCH images have the first 16 bytes reserved, and the 4-bytes signature starts
// immediately after. Older images (ICH8/9/10) have the signature at the
// beginning.
// TODO: Check this. What if we have the signature in both places? I feel like the check
// should be IsICH because I expect the ICH to override PCH if the signature exists in 0:4
// since in that case 16:20 should be data. If that's the case, FindSignature needs to
// be fixed as well
func (f *FlashImage) IsPCH() bool {
	return bytes.Equal(f.buf[16:16+len(FlashSignature)], FlashSignature)
}

// FindSignature looks for the Intel flash signature, and returns its offset
// from the start of the image. The PCH images are located at offset 16, while
// in ICH8/9/10 they start at 0. If no signature is found, it returns -1.
func (f *FlashImage) FindSignature() (int, error) {
	return FindSignature(f.buf)
}

func (f *FlashImage) String() string {
	return fmt.Sprintf("FlashImage{Size=%v, Descriptor=%v, Region=%v, Master=%v}",
		len(f.buf),
		f.IFD.DescriptorMap.String(),
		f.IFD.Region.String(),
		f.IFD.Master.String(),
	)
}

func (f *FlashImage) fillRegionGaps() error {
	// Search for gaps and fill in with unknown regions
	offset := uint64(FlashDescriptorLength)
	var newRegions []*TypedFirmware
	for _, t := range f.Regions {
		r, _ := t.Value.(Region)
		nextBase := uint64(r.FlashRegion().BaseOffset())
		if nextBase < offset {
			// Something is wrong, overlapping regions
			// TODO: print a better error message describing what it overlaps with
			return fmt.Errorf("overlapping regions! region type %s overlaps with the previous region",
				r.Type().String())
		}
		if nextBase > offset {
			// There is a gap, create an unknown region
			tempFR := &FlashRegion{Base: uint16(offset / RegionBlockSize),
				Limit: uint16(nextBase/RegionBlockSize) - 1}
			newRegions = append(newRegions, MakeTyped(&RawRegion{buf: f.buf[offset:nextBase],
				FRegion:    tempFR,
				RegionType: RegionTypeUnknown}))
		}
		offset = uint64(r.FlashRegion().EndOffset())
		newRegions = append(newRegions, MakeTyped(r))
	}
	// check for the last region
	if offset != f.FlashSize {
		tempFR := &FlashRegion{Base: uint16(offset / RegionBlockSize),
			Limit: uint16(f.FlashSize/RegionBlockSize) - 1}
		newRegions = append(newRegions, MakeTyped(&RawRegion{buf: f.buf[offset:f.FlashSize],
			FRegion:    tempFR,
			RegionType: RegionTypeUnknown}))
	}
	f.Regions = newRegions
	return nil
}

// NewFlashImage tries to create a FlashImage structure, and returns a FlashImage
// and an error if any. This only works with images that operate in Descriptor
// mode.
func NewFlashImage(buf []byte) (*FlashImage, error) {
	if len(buf) < FlashDescriptorLength {
		return nil, fmt.Errorf("NewFlashImage: need at least %d bytes, only %d provided:%w", FlashDescriptorLength, len(buf), ErrTooShort)
	}
	f := FlashImage{FlashSize: uint64(len(buf))}

	// Copy out buffers
	f.buf = make([]byte, len(buf))
	copy(f.buf, buf)
	f.IFD.buf = make([]byte, FlashDescriptorLength)
	copy(f.IFD.buf, buf[:FlashDescriptorLength])

	if err := f.IFD.ParseFlashDescriptor(); err != nil {
		return nil, err
	}

	// FlashRegions is an array, make a slice to keep reference to it's content
	frs := f.IFD.Region.FlashRegions[:]

	// BIOS region has to be valid
	if !frs[RegionTypeBIOS].Valid() {
		return nil, fmt.Errorf("no BIOS region: invalid region parameters %v", frs[RegionTypeBIOS])
	}

	nr := int(f.IFD.DescriptorMap.NumberOfRegions)
	// Parse all the regions
	for i, fr := range frs {
		// Parse only a smaller number of regions if number of regions isn't 0
		// Number of regions is deprecated in newer IFDs and is just 0, older IFDs report
		// the number of regions and have falsely "valid" regions after that number.
		if nr != 0 && i >= nr {
			break
		}
		if !fr.Valid() {
			continue
		}
		if o := uint64(fr.BaseOffset()); o >= f.FlashSize {
			fmt.Printf("region %s (%d, %v) out of bounds: BaseOffset %#x, Flash size %#x, skipping...\n",
				flashRegionTypeNames[FlashRegionType(i)], i, fr, o, f.FlashSize)
			continue
		}
		if o := uint64(fr.EndOffset()); o > f.FlashSize {
			fmt.Printf("region %s (%d, %v) out of bounds: EndOffset %#x, Flash size %#x, skipping...\n",
				flashRegionTypeNames[FlashRegionType(i)], i, fr, o, f.FlashSize)
			continue
		}
		if c, ok := regionConstructors[FlashRegionType(i)]; ok {
			r, err := c(buf[fr.BaseOffset():fr.EndOffset()], &frs[i], FlashRegionType(i))
			if err != nil {
				return nil, err
			}
			f.Regions = append(f.Regions, MakeTyped(r))
		}
	}

	// Sort the regions by offset so we can look for gaps
	sort.Slice(f.Regions, func(i, j int) bool {
		ri := f.Regions[i].Value.(Region)
		rj := f.Regions[j].Value.(Region)
		return ri.FlashRegion().Base < rj.FlashRegion().Base
	})

	if err := f.fillRegionGaps(); err != nil {
		return nil, err
	}
	return &f, nil
}
