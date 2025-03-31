package uefi

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinytoy-sec/UefiVarMonitor/pkg/guid"
	"github.com/tinytoy-sec/UefiVarMonitor/pkg/log"
)

// FVFileType 表示EFI文件中可能的不同类型
type FVFileType uint8

// UEFI FV 文件类型
const (
	FVFileTypeAll FVFileType = iota
	FVFileTypeRaw
	FVFileTypeFreeForm
	FVFileTypeSECCore
	FVFileTypePEICore
	FVFileTypeDXECore
	FVFileTypePEIM
	FVFileTypeDriver
	FVFileTypeCombinedPEIMDriver
	FVFileTypeApplication
	FVFileTypeSMM
	FVFileTypeVolumeImage
	FVFileTypeCombinedSMMDXE
	FVFileTypeSMMCore
	FVFileTypeSMMStandalone
	FVFileTypeSMMCoreStandalone
	FVFileTypeOEMMin   FVFileType = 0xC0
	FVFileTypeOEMMax   FVFileType = 0xDF
	FVFileTypeDebugMin FVFileType = 0xE0
	FVFileTypeDebugMax FVFileType = 0xEF
	FVFileTypePad      FVFileType = 0xF0
	FVFileTypeFFSMin   FVFileType = 0xF0
	FVFileTypeFFSMax   FVFileType = 0xFF
)

// SupportedFiles 是将被解析的文件类型列表。不在此列表上的文件类型被视为不透明的二进制blob。
var SupportedFiles = map[FVFileType]bool{
	// 这些是我们实际尝试解析部分的文件类型
	FVFileTypeRaw:      false,
	FVFileTypeFreeForm: true,
	FVFileTypeSECCore:  true,
	FVFileTypePEICore:  true,
	FVFileTypeDXECore:  true,
	// 注意：注释掉此行可防止PEI模块被解压。这解决了PEI在重新压缩时过大的问题。
	//FVFileTypePEIM:               true,
	FVFileTypeDriver:             true,
	FVFileTypeCombinedPEIMDriver: true,
	FVFileTypeApplication:        true,
	FVFileTypeSMM:                true,
	FVFileTypeVolumeImage:        true,
	FVFileTypeCombinedSMMDXE:     true,
	FVFileTypeSMMCore:            true,
	FVFileTypeSMMStandalone:      true,
	FVFileTypeSMMCoreStandalone:  true,
}

var fileTypeNames = map[FVFileType]string{
	FVFileTypeRaw:                "EFI_FV_FILETYPE_RAW",
	FVFileTypeFreeForm:           "EFI_FV_FILETYPE_FREEFORM",
	FVFileTypeSECCore:            "EFI_FV_FILETYPE_SECURITY_CORE",
	FVFileTypePEICore:            "EFI_FV_FILETYPE_PEI_CORE",
	FVFileTypeDXECore:            "EFI_FV_FILETYPE_DXE_CORE",
	FVFileTypePEIM:               "EFI_FV_FILETYPE_PEIM",
	FVFileTypeDriver:             "EFI_FV_FILETYPE_DRIVER",
	FVFileTypeCombinedPEIMDriver: "EFI_FV_FILETYPE_COMBINED_PEIM_DRIVER",
	FVFileTypeApplication:        "EFI_FV_FILETYPE_APPLICATION",
	FVFileTypeSMM:                "EFI_FV_FILETYPE_MM",
	FVFileTypeVolumeImage:        "EFI_FV_FILETYPE_FIRMWARE_VOLUME_IMAGE",
	FVFileTypeCombinedSMMDXE:     "EFI_FV_FILETYPE_COMBINED_MM_DXE",
	FVFileTypeSMMCore:            "EFI_FV_FILETYPE_MM_CORE",
	FVFileTypeSMMStandalone:      "EFI_FV_FILETYPE_MM_STANDALONE",
	FVFileTypeSMMCoreStandalone:  "EFI_FV_FILETYPE_MM_CORE_STANDALONE",
}

// NamesToFileType 将常见文件类型字符串映射到实际类型
var NamesToFileType map[string]FVFileType

func init() {
	NamesToFileType = make(map[string]FVFileType)
	for k, v := range fileTypeNames {
		NamesToFileType[strings.TrimPrefix(v, "EFI_FV_FILETYPE_")] = k
	}
}

// String 为文件类型创建字符串表示
func (f FVFileType) String() string {
	if s, ok := fileTypeNames[f]; ok {
		return s
	}
	return fmt.Sprintf("UNKNOWN_FILETYPE_%#x", int(f))
}

// Stock GUIDS
var (
	ZeroGUID = guid.MustParse("00000000-0000-0000-0000-000000000000")
	FFGUID   = guid.MustParse("FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF")
)

// FileAlignments specifies the correct alignments based on the field in the file header.
var fileAlignments = []uint64{
	// These alignments are not computable, we have to look them up.
	1,
	16,
	128,
	512,
	1024,
	4 * 1024,
	32 * 1024,
	64 * 1024,
	128 * 1024,
	256 * 1024,
	512 * 1024,
	1024 * 1024,
	2 * 1024 * 1024,
	4 * 1024 * 1024,
	8 * 1024 * 1024,
	16 * 1024 * 1024,
}

const (
	// FileHeaderMinLength is the minimum length of a firmware file header.
	FileHeaderMinLength = 0x18
	// FileHeaderExtMinLength is the minimum length of an extended firmware file header.
	FileHeaderExtMinLength = 0x20
	// EmptyBodyChecksum is the value placed in the File IntegrityCheck field if the body checksum bit isn't set.
	EmptyBodyChecksum uint8 = 0xAA
)

// IntegrityCheck holds the two 8 bit checksums for the file header and body separately.
type IntegrityCheck struct {
	Header uint8
	File   uint8
}

type fileAttr uint8

// FileState (needs to be xored with Attributes.ErasePolarity)
type FileState uint8

// File State Bits
const (
	FileStateHeaderConstruction FileState = 0x01
	FileStateHeaderValid        FileState = 0x02
	FileStateDataValid          FileState = 0x04
	FileStateMarkeForUpdate     FileState = 0x08
	FileStateDeleted            FileState = 0x10
	FileStateHeaderInvalid      FileState = 0x20

	FileStateValid FileState = FileStateHeaderConstruction | FileStateHeaderValid | FileStateDataValid
)

type ThreeUint8 [3]uint8

func (t *ThreeUint8) UnmarshalJSON(b []byte) error {
	if copy(t[:], b) == 0 {
		return fmt.Errorf("cannot unmarshal 3 uint8 from %v", b)
	}
	return nil
}

func (t *ThreeUint8) MarshalJSON() ([]byte, error) {
	res := Read3Size(*t)
	return json.Marshal(res)
}

// FileHeader represents an EFI File header.
type FileHeader struct {
	GUID       guid.GUID      // This is the GUID of the file.
	Checksum   IntegrityCheck `json:"-"`
	Type       FVFileType
	Attributes fileAttr
	Size       ThreeUint8
	State      FileState
}

// IsLarge checks if the large file attribute is set.
func (a fileAttr) IsLarge() bool {
	return a&0x01 != 0
}

// GetAlignment returns the byte alignment specified by the file header.
func (a fileAttr) GetAlignment() uint64 {
	alignVal := (a & 0x38) >> 3
	alignVal |= (a & 0x02) << 2
	return fileAlignments[alignVal]
}

// Sets the large file attribute.
func (a *fileAttr) setLarge(large bool) {
	if large {
		*a |= 0x01
	} else {
		*a &= 0xFE
	}
}

// HasChecksum checks if we need to checksum the file body.
func (a fileAttr) HasChecksum() bool {
	return a&0x40 != 0
}

// SetState sets file state respecting erase polarity
func (fh *FileHeader) SetState(s FileState) {
	fh.State = s ^ FileState(Attributes.ErasePolarity)
}

// HeaderLen returns the length of the file header depending on the file size.
func (f *File) HeaderLen() uint64 {
	if f.Header.Attributes.IsLarge() {
		return FileHeaderExtMinLength
	}
	return FileHeaderMinLength
}

// ChecksumHeader returns a checksum of the header.
func (f *File) ChecksumHeader() uint8 {
	fh := f.Header
	headerSize := FileHeaderMinLength
	if fh.Attributes.IsLarge() {
		headerSize = FileHeaderExtMinLength
	}
	// Sum over header without State and IntegrityCheck.File.
	// To do that we just sum over the whole header and subtract.
	// UEFI PI Spec 3.2.3 EFI_FFS_FILE_HEADER
	sum := Checksum8(f.buf[:headerSize])
	sum -= fh.Checksum.File
	sum -= uint8(fh.State)
	return sum
}

// FileHeaderExtended represents an EFI File header with the
// large file attribute set.
// We also use this as the generic header for all EFI files, regardless of whether
// they are actually large. This makes it easier for us to just return one type
// All sizes are also copied into the ExtendedSize field so we only have to check once
type FileHeaderExtended struct {
	FileHeader
	ExtendedSize uint64 `json:"-"`
}

// File represents an EFI File.
type File struct {
	Header FileHeaderExtended
	Type   string

	// a File can contain either Sections or an NVarStore but not both
	Sections  []*Section `json:",omitempty"`
	NVarStore *NVarStore `json:",omitempty"`

	buf         []byte
	ExtractPath string
	DataOffset  uint64
}

func (f *File) Buf() []byte {
	return f.buf
}

func (f *File) SetBuf(buf []byte) {
	f.buf = buf
}

// Apply calls the visitor on the File.
func (f *File) Apply(v Visitor) error {
	return v.Visit(f)
}

func (f *File) ApplyChildren(v Visitor) error {
	if f.NVarStore != nil {
		if err := f.NVarStore.Apply(v); err != nil {
			return err
		}
		return nil
	}
	for _, s := range f.Sections {
		if err := s.Apply(v); err != nil {
			return err
		}
	}
	return nil
}

func (f *File) SetSize(size uint64, resizeFile bool) {
	fh := &f.Header
	// See if we need the extended size
	// Check if size > 3 bytes size field
	fh.ExtendedSize = size
	fh.Attributes.setLarge(false)
	if fh.ExtendedSize > 0xFFFFFF {
		// Can't fit, need extended header
		if resizeFile {
			// Increase the file size by the additional space needed
			// for the extended header.
			fh.ExtendedSize += FileHeaderExtMinLength - FileHeaderMinLength
		}
		fh.Attributes.setLarge(true)
	}
	// This will set size to 0xFFFFFF if too big.
	fh.Size = Write3Size(fh.ExtendedSize)
}

func (f *File) ChecksumAndAssemble(fileData []byte) error {

	fh := &f.Header

	header := new(bytes.Buffer)
	err := binary.Write(header, binary.LittleEndian, fh)
	if err != nil {
		return fmt.Errorf("unable to construct binary header of file %v, got %v",
			fh.GUID, err)
	}
	f.buf = header.Bytes()
	// We need to get rid of whatever it sums to so that the overall sum is zero
	// Sorry about the name :(
	fh.Checksum.Header -= f.ChecksumHeader()

	// Checksum the body
	fh.Checksum.File = EmptyBodyChecksum
	if fh.Attributes.HasChecksum() {
		// if the empty checksum had been set to 0 instead of 0xAA
		// this could have been a bit nicer. BUT NOOOOOOO.
		fh.Checksum.File = 0 - Checksum8(fileData)
	}

	header = new(bytes.Buffer)
	if fh.Attributes.IsLarge() {
		err = binary.Write(header, binary.LittleEndian, fh)
	} else {
		err = binary.Write(header, binary.LittleEndian, fh.FileHeader)
	}
	if err != nil {
		return err
	}
	f.buf = header.Bytes()

	f.buf = append(f.buf, fileData...)
	return nil
}

// CreatePadFile creates an empty pad file in order to align the next file.
func CreatePadFile(size uint64) (*File, error) {
	if size < FileHeaderMinLength {
		return nil, fmt.Errorf("size too small! min size required is %#x bytes, requested %#x",
			FileHeaderMinLength, size)
	}

	f := File{}
	fh := &f.Header

	// Create empty guid
	if Attributes.ErasePolarity == 0xFF {
		fh.GUID = *FFGUID
	} else if Attributes.ErasePolarity == 0 {
		fh.GUID = *ZeroGUID
	} else {
		return nil, fmt.Errorf("erase polarity not 0x00 or 0xFF, got %#x", Attributes.ErasePolarity)
	}

	// TODO: I see examples of this where the attributes are just 0 and not dependent on the
	// erase polarity. Is that right? Check and handle.
	fh.Attributes = 0

	// Set the size. If the file is too big, we take up more of the padding for the header.
	// This also sets the large file attribute if file is big.
	f.SetSize(size, false)
	fh.Type = FVFileTypePad
	f.Type = fh.Type.String()

	// Create empty pad filedata based on size
	var fileData []byte
	fileData = make([]byte, size-FileHeaderMinLength)
	if fh.Attributes.IsLarge() {
		fileData = make([]byte, size-FileHeaderExtMinLength)
	}
	// Fill with empty bytes
	for i, dataLen := 0, len(fileData); i < dataLen; i++ {
		fileData[i] = Attributes.ErasePolarity
	}

	fh.SetState(FileStateValid)

	// Everything has been setup. Checksum and create.
	if err := f.ChecksumAndAssemble(fileData); err != nil {
		return nil, err
	}
	return &f, nil
}

func NewFile(buf []byte) (*File, error) {
	f := File{}
	f.DataOffset = FileHeaderMinLength
	// Read in standard header.
	r := bytes.NewReader(buf)
	if err := binary.Read(r, binary.LittleEndian, &f.Header.FileHeader); err != nil {
		return nil, err
	}

	// Map type to string.
	f.Type = f.Header.Type.String()

	// TODO: Check Attribute flag as well. How important is the attribute flag? we already
	// have FFFFFF in the size
	if f.Header.Size == [3]uint8{0xFF, 0xFF, 0xFF} {
		// Extended Header
		if err := binary.Read(r, binary.LittleEndian, &f.Header.ExtendedSize); err != nil {
			return nil, err
		}
		if f.Header.ExtendedSize == 0xFFFFFFFFFFFFFFFF {
			// Start of free space
			// Note: this is not a pad file. Pad files also have valid headers.
			return nil, nil
		}
		f.DataOffset = FileHeaderExtMinLength
	} else {
		// Copy small size into big for easier handling.
		// Damn the 3 byte sizes.
		f.Header.ExtendedSize = Read3Size(f.Header.Size)
	}

	if buflen := len(buf); f.Header.ExtendedSize > uint64(buflen) {
		return nil, fmt.Errorf("File size too big! File with GUID: %v has length %v, but is only %v bytes big",
			f.Header.GUID, f.Header.ExtendedSize, buflen)
	}

	if ReadOnly {
		f.buf = buf[:f.Header.ExtendedSize]
	} else {
		// Copy out the buffer.
		newBuf := buf[:f.Header.ExtendedSize]
		f.buf = make([]byte, f.Header.ExtendedSize)
		copy(f.buf, newBuf)
	}

	// Special case for NVAR Store stored in raw file
	if f.Header.Type == FVFileTypeRaw && f.Header.GUID == *NVAR {
		if f.DataOffset >= uint64(len(f.buf)) {
			return nil, fmt.Errorf("data offset %#x exceeds buffer size %#x", f.DataOffset, len(f.buf))
		}
		ns, err := NewNVarStore(f.buf[f.DataOffset:])
		if err != nil {
			log.Errorf("error parsing NVAR store in file %v: %v", f.Header.GUID, err)
		}
		// Note that ns is nil if there was an error, so this assign is fine either way.
		f.NVarStore = ns
	}

	// Parse sections
	if !SupportedFiles[f.Header.Type] {
		return &f, nil
	}

	for i, offset := 0, f.DataOffset; offset < f.Header.ExtendedSize; i++ {
		s, err := NewSection(f.buf[offset:], i)
		if err != nil {
			return nil, fmt.Errorf("error parsing sections of file %v: %v", f.Header.GUID, err)
		}
		if s.Header.ExtendedSize == 0 {
			return nil, fmt.Errorf("invalid length of section of file %v", f.Header.GUID)
		}
		offset += uint64(s.Header.ExtendedSize)
		// Align to 4 bytes for now. The PI Spec doesn't say what alignment it should be
		// but UEFITool aligns to 4 bytes, and this seems to work on everything I have.
		offset = Align4(offset)
		f.Sections = append(f.Sections, s)
	}
	return &f, nil
}

// Checksum8 计算给定字节切片的8位校验和
// 按照UEFI PI规范计算校验和
func Checksum8(data []byte) uint8 {
	var sum uint8
	for _, d := range data {
		sum += d
	}
	return sum
}
