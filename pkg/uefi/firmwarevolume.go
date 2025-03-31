package uefi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/tinytoy-sec/UefiVarMonitor/pkg/guid"
	"github.com/tinytoy-sec/UefiVarMonitor/pkg/log"
)

// FirmwareVolume 常量
const (
	FirmwareVolumeFixedHeaderSize  = 56
	FirmwareVolumeMinSize          = FirmwareVolumeFixedHeaderSize + 8 // +8 用于终止块列表的空块
	FirmwareVolumeExtHeaderMinSize = 20
)

// 有效的FV GUID
var (
	FFS1      = guid.MustParse("7a9354d9-0468-444a-81ce-0bf617d890df")
	FFS2      = guid.MustParse("8c8ce578-8a3d-4f1c-9935-896185c32dd3")
	FFS3      = guid.MustParse("5473c07a-3dcb-4dca-bd6f-1e9689e7349a")
	EVSA      = guid.MustParse("fff12b8d-7696-4c8b-a985-2747075b4f50")
	NVAR      = guid.MustParse("cef5b9a3-476d-497f-9fdc-e98143e0422c")
	EVSA2     = guid.MustParse("00504624-8a59-4eeb-bd0f-6b36e96128e0")
	AppleBoot = guid.MustParse("04adeead-61ff-4d31-b6ba-64f8bf901f5a")
	PFH1      = guid.MustParse("16b45da2-7d70-4aea-a58d-760e9ecb841d")
	PFH2      = guid.MustParse("e360bdba-c3ce-46be-8f37-b231e5cb9f35")
)

// FVGUIDs 保存常见FV类型名称
var FVGUIDs = map[guid.GUID]string{
	*FFS1:      "FFS1",
	*FFS2:      "FFS2",
	*FFS3:      "FFS3",
	*EVSA:      "NVRAM_EVSA",
	*NVAR:      "NVRAM_NVAR",
	*EVSA2:     "NVRAM_EVSA2",
	*AppleBoot: "APPLE_BOOT",
	*PFH1:      "PFH1",
	*PFH2:      "PFH2",
}

// 这些是我们实际尝试解析的FV，超出头部范围
// 我们只解析FFS2和FFS3
var supportedFVs = map[guid.GUID]bool{
	*FFS2: true,
	*FFS3: true,
}

// Block 描述固件卷块的数量和大小
type Block struct {
	Count uint32
	Size  uint32
}

// FirmwareVolumeFixedHeader 包含固件卷头的固定字段
type FirmwareVolumeFixedHeader struct {
	_               [16]uint8
	FileSystemGUID  guid.GUID
	Length          uint64
	Signature       uint32
	Attributes      uint32 // UEFI PI规范卷3.2.1 EFI_FIRMWARE_VOLUME_HEADER
	HeaderLen       uint16
	Checksum        uint16
	ExtHeaderOffset uint16
	Reserved        uint8 `json:"-"`
	Revision        uint8
	// _               [3]uint8
}

// FirmwareVolumeExtHeader 包含扩展固件卷头的字段
type FirmwareVolumeExtHeader struct {
	FVName        guid.GUID
	ExtHeaderSize uint32
}

// FirmwareVolume 表示固件卷。它结合了固定头和可变块列表
type FirmwareVolume struct {
	FirmwareVolumeFixedHeader
	// 必须至少有一个为零并指示块列表的结束
	// 我们不必真正关心块，因为我们只是读取所有内容
	Blocks []Block
	FirmwareVolumeExtHeader
	Files []*File `json:",omitempty"`

	// 二进制中没有的变量，用于跟踪/打印
	DataOffset  uint64
	FVType      string `json:"-"`
	buf         []byte
	FVOffset    uint64 // 从BIOS区域起始处的字节偏移量
	ExtractPath string
	Resizable   bool   // 确定这个FV是否可调整大小
	FreeSpace   uint64 `json:"-"`
}

// Buf 返回缓冲区
// 主要用于与Firmware接口交互的东西
func (fv *FirmwareVolume) Buf() []byte {
	return fv.buf
}

// SetBuf sets the buffer.
// Used mostly for things interacting with the Firmware interface.
func (fv *FirmwareVolume) SetBuf(buf []byte) {
	fv.buf = buf
}

// Apply calls the visitor on the FirmwareVolume.
func (fv *FirmwareVolume) Apply(v Visitor) error {
	return v.Visit(fv)
}

// ApplyChildren calls the visitor on each child node of FirmwareVolume.
func (fv *FirmwareVolume) ApplyChildren(v Visitor) error {
	for _, f := range fv.Files {
		if err := f.Apply(v); err != nil {
			return err
		}
	}
	return nil
}

// GetErasePolarity gets the erase polarity
func (fv *FirmwareVolume) GetErasePolarity() uint8 {
	if fv.Attributes&0x800 != 0 {
		return 0xFF
	}
	return 0
}

// String creates a string representation for the firmware volume.
func (fv FirmwareVolume) String() string {
	if fv.ExtHeaderOffset != 0 {
		return fv.FVName.String()
	}
	return fv.FileSystemGUID.String()
}

// InsertFile appends the file to the end of the buffer according to alignment requirements.
func (fv *FirmwareVolume) InsertFile(alignedOffset uint64, fBuf []byte) error {
	// fv.Length should contain the minimum fv size.
	// If Resizable is not set, this is the exact FV size.
	bufLen := uint64(len(fv.buf))
	if bufLen > alignedOffset {
		return fmt.Errorf("aligned offset is in the middle of the FV, offset was %#x, fv buffer was %#x",
			alignedOffset, bufLen)
	}

	// add padding for alignment
	for i, num := uint64(0), alignedOffset-bufLen; i < num; i++ {
		fv.buf = append(fv.buf, Attributes.ErasePolarity)
	}

	// Check size
	fLen := uint64(len(fBuf))
	if fLen == 0 {
		return errors.New("trying to insert empty file")
	}
	// Overwrite old data in the firmware volume.
	fv.buf = append(fv.buf, fBuf...)
	return nil
}

// FindFirmwareVolumeOffset searches for a firmware volume signature, "_FVH"
// using 8-byte alignment. If found, returns the offset from the start of the
// bios region, otherwise returns -1.
func FindFirmwareVolumeOffset(data []byte) int64 {
	if len(data) < 32 {
		return -1
	}
	var (
		offset int64
		fvSig  = []byte("_FVH")
	)
	for offset = 32; offset+4 < int64(len(data)); offset += 8 {
		if bytes.Equal(data[offset:offset+4], fvSig) {
			return offset - 40 // the actual volume starts 40 bytes before the signature
		}
	}
	return -1
}

// NewFirmwareVolume parses a sequence of bytes and returns a FirmwareVolume
// object, if a valid one is passed, or an error
func NewFirmwareVolume(data []byte, fvOffset uint64, resizable bool) (*FirmwareVolume, error) {
	fv := FirmwareVolume{Resizable: resizable}

	if len(data) < FirmwareVolumeMinSize {
		return nil, fmt.Errorf("Firmware Volume size too small: expected %v bytes, got %v",
			FirmwareVolumeMinSize,
			len(data),
		)
	}
	reader := bytes.NewReader(data)
	if err := binary.Read(reader, binary.LittleEndian, &fv.FirmwareVolumeFixedHeader); err != nil {
		return nil, err
	}
	// read the block map
	blocks := make([]Block, 0)
	for {
		var block Block
		if err := binary.Read(reader, binary.LittleEndian, &block); err != nil {
			return nil, err
		}
		if block.Count == 0 && block.Size == 0 {
			// found the terminating block
			break
		}
		blocks = append(blocks, block)
	}
	fv.Blocks = blocks

	// Set the erase polarity
	if err := SetErasePolarity(fv.GetErasePolarity()); err != nil {
		return nil, err
	}

	// Boundary checks (to return an error instead of panicking)
	if fv.Length > uint64(len(data)) {
		return nil, fmt.Errorf("invalid FV length (is greater than the data length): %d > %d",
			fv.Length, len(data))
	}

	// Parse the extended header and figure out the start of data
	fv.DataOffset = uint64(fv.HeaderLen)
	if fv.ExtHeaderOffset != 0 &&
		fv.Length >= FirmwareVolumeExtHeaderMinSize &&
		uint64(fv.ExtHeaderOffset) < fv.Length-FirmwareVolumeExtHeaderMinSize {

		// jump to ext header offset.
		r := bytes.NewReader(data[fv.ExtHeaderOffset:])
		if err := binary.Read(r, binary.LittleEndian, &fv.FirmwareVolumeExtHeader); err != nil {
			return nil, fmt.Errorf("unable to parse FV extended header, got: %v", err)
		}
		// TODO: will the ext header ever end before the regular header? I don't believe so. Add a check?
		fv.DataOffset = uint64(fv.ExtHeaderOffset) + uint64(fv.ExtHeaderSize)
	}
	// Make sure DataOffset is 8 byte aligned at least.
	// TODO: handle alignment field in header.
	fv.DataOffset = Align8(fv.DataOffset)

	fv.FVType = FVGUIDs[fv.FileSystemGUID]
	fv.FVOffset = fvOffset

	if ReadOnly {
		fv.buf = data[:fv.Length]
	} else {
		// copy out the buffer.
		newBuf := data[:fv.Length]
		fv.buf = make([]byte, fv.Length)
		copy(fv.buf, newBuf)
	}

	// Parse the files.
	// TODO: handle fv data alignment.
	// Start from the end of the fv header.
	// Test if the fv type is supported.
	if _, ok := supportedFVs[fv.FileSystemGUID]; !ok {
		log.Warnf("unsupported fv type %v,%v not parsing it", fv.FileSystemGUID.String(), fv.FVType)
		return &fv, nil
	}
	lh := fv.Length - FileHeaderMinLength
	var prevLen uint64
	for offset := fv.DataOffset; offset < lh; offset += prevLen {
		offset = Align8(offset)
		if uint64(len(data)) <= offset {
			return nil, fmt.Errorf("offset %#x is beyond end of FV data (%#x)", offset, len(data))
		}
		file, err := NewFile(data[offset:])
		if err != nil {
			return nil, fmt.Errorf("unable to construct firmware file at offset %#x into FV: %v", offset, err)
		}
		if file == nil {
			// We've reached free space. Terminate
			fv.FreeSpace = fv.Length - offset
			break
		}
		fv.Files = append(fv.Files, file)
		prevLen = file.Header.ExtendedSize
		if prevLen == 0 {
			return nil, fmt.Errorf("invalid length of file at offset %#x", offset)
		}
	}
	return &fv, nil
}
