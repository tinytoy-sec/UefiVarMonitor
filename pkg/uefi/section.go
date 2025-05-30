package uefi

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"unsafe"

	"github.com/tinytoy-sec/UefiVarMonitor/pkg/compression"
	"github.com/tinytoy-sec/UefiVarMonitor/pkg/guid"
	"github.com/tinytoy-sec/UefiVarMonitor/pkg/log"
	"github.com/tinytoy-sec/UefiVarMonitor/pkg/unicode"
)

const (
	// 文件段头的最小长度
	SectionMinLength = 0x04
	// 扩展文件段头的最小长度
	SectionExtMinLength = 0x08
)

// SectionType 保存段类型值
type SectionType uint8

// UEFI 段类型常量
const (
	SectionTypeAll                 SectionType = 0x00
	SectionTypeCompression         SectionType = 0x01
	SectionTypeGUIDDefined         SectionType = 0x02
	SectionTypeDisposable          SectionType = 0x03
	SectionTypePE32                SectionType = 0x10
	SectionTypePIC                 SectionType = 0x11
	SectionTypeTE                  SectionType = 0x12
	SectionTypeDXEDepEx            SectionType = 0x13
	SectionTypeVersion             SectionType = 0x14
	SectionTypeUserInterface       SectionType = 0x15
	SectionTypeCompatibility16     SectionType = 0x16
	SectionTypeFirmwareVolumeImage SectionType = 0x17
	SectionTypeFreeformSubtypeGUID SectionType = 0x18
	SectionTypeRaw                 SectionType = 0x19
	SectionTypePEIDepEx            SectionType = 0x1b
	SectionMMDepEx                 SectionType = 0x1c
)

// 段类型名称映射
var sectionTypeNames = map[SectionType]string{
	SectionTypeCompression:         "EFI_SECTION_COMPRESSION",
	SectionTypeGUIDDefined:         "EFI_SECTION_GUID_DEFINED",
	SectionTypeDisposable:          "EFI_SECTION_DISPOSABLE",
	SectionTypePE32:                "EFI_SECTION_PE32",
	SectionTypePIC:                 "EFI_SECTION_PIC",
	SectionTypeTE:                  "EFI_SECTION_TE",
	SectionTypeDXEDepEx:            "EFI_SECTION_DXE_DEPEX",
	SectionTypeVersion:             "EFI_SECTION_VERSION",
	SectionTypeUserInterface:       "EFI_SECTION_USER_INTERFACE",
	SectionTypeCompatibility16:     "EFI_SECTION_COMPATIBILITY16",
	SectionTypeFirmwareVolumeImage: "EFI_SECTION_FIRMWARE_VOLUME_IMAGE",
	SectionTypeFreeformSubtypeGUID: "EFI_SECTION_FREEFORM_SUBTYPE_GUID",
	SectionTypeRaw:                 "EFI_SECTION_RAW",
	SectionTypePEIDepEx:            "EFI_SECTION_PEI_DEPEX",
	SectionMMDepEx:                 "EFI_SECTION_MM_DEPEX",
}

// String creates a string representation for the section type.
func (s SectionType) String() string {
	if t, ok := sectionTypeNames[s]; ok {
		return t
	}
	return "UNKNOWN"
}

// GUIDEDSectionAttribute holds a GUIDED section attribute bitfield
type GUIDEDSectionAttribute uint16

// UEFI GUIDED Section Attributes
const (
	GUIDEDSectionProcessingRequired GUIDEDSectionAttribute = 0x01
	GUIDEDSectionAuthStatusValid    GUIDEDSectionAttribute = 0x02
)

// SectionHeader represents an EFI_COMMON_SECTION_HEADER as specified in
// UEFI PI Spec 3.2.4 Firmware File Section
type SectionHeader struct {
	Size [3]uint8 `json:"-"`
	Type SectionType
}

// SectionExtHeader represents an EFI_COMMON_SECTION_HEADER2 as specified in
// UEFI PI Spec 3.2.4 Firmware File Section
type SectionExtHeader struct {
	SectionHeader
	ExtendedSize uint32 `json:"-"`
}

// SectionGUIDDefinedHeader contains the fields for a EFI_SECTION_GUID_DEFINED
// encapsulated section header.
type SectionGUIDDefinedHeader struct {
	GUID       guid.GUID
	DataOffset uint16
	Attributes uint16
}

// SectionGUIDDefined contains the type specific fields for a
// EFI_SECTION_GUID_DEFINED section.
type SectionGUIDDefined struct {
	SectionGUIDDefinedHeader

	// Metadata
	Compression string
}

// GetBinHeaderLen returns the length of the binary typ specific header
func (s *SectionGUIDDefined) GetBinHeaderLen() uint32 {
	return uint32(unsafe.Sizeof(s.SectionGUIDDefinedHeader))
}

// TypeHeader interface forces type specific headers to report their length
type TypeHeader interface {
	GetBinHeaderLen() uint32
}

// TypeSpecificHeader is used for marshalling and unmarshalling from JSON
type TypeSpecificHeader struct {
	Type   SectionType
	Header TypeHeader
}

var headerTypes = map[SectionType]func() TypeHeader{
	SectionTypeGUIDDefined: func() TypeHeader { return &SectionGUIDDefined{} },
}

// UnmarshalJSON unmarshals a TypeSpecificHeader struct and correctly deduces the
// type of the interface.
func (t *TypeSpecificHeader) UnmarshalJSON(b []byte) error {
	var getType struct {
		Type   SectionType
		Header json.RawMessage
	}
	if err := json.Unmarshal(b, &getType); err != nil {
		return err
	}
	factory, ok := headerTypes[getType.Type]
	if !ok {
		return fmt.Errorf("unknown TypeSpecificHeader type '%v', unable to unmarshal", getType.Type)
	}
	t.Type = SectionType(getType.Type)
	t.Header = factory()
	return json.Unmarshal(getType.Header, &t.Header)
}

// DepExOpCode is one opcode for the dependency expression section.
type DepExOpCode string

// DepExOpCodes maps the numeric code to the string.
var DepExOpCodes = map[byte]DepExOpCode{
	0x0: "BEFORE",
	0x1: "AFTER",
	0x2: "PUSH",
	0x3: "AND",
	0x4: "OR",
	0x5: "NOT",
	0x6: "TRUE",
	0x7: "FALSE",
	0x8: "END",
	0x9: "SOR",
}

// DepExNamesToOpCodes maps the operation back to the code.
var DepExNamesToOpCodes = map[DepExOpCode]byte{}

func init() {
	for k, v := range DepExOpCodes {
		DepExNamesToOpCodes[v] = k
	}
}

// DepExOp contains one operation for the dependency expression.
type DepExOp struct {
	OpCode DepExOpCode
	GUID   *guid.GUID `json:",omitempty"`
}

// Section represents a Firmware File Section
type Section struct {
	Header SectionExtHeader
	Type   string
	buf    []byte

	// Metadata for extraction and recovery
	ExtractPath string
	FileOrder   int `json:"-"`

	// Type specific fields
	// TODO: It will be simpler if this was not an interface
	TypeSpecific *TypeSpecificHeader `json:",omitempty"`

	// For EFI_SECTION_USER_INTERFACE
	Name string `json:",omitempty"`

	// For EFI_SECTION_VERSION
	BuildNumber uint16 `json:",omitempty"`
	Version     string `json:",omitempty"`

	// For EFI_SECTION_DXE_DEPEX, EFI_SECTION_PEI_DEPEX, and EFI_SECTION_MM_DEPEX
	DepEx []DepExOp `json:",omitempty"`

	// Encapsulated firmware
	Encapsulated []*TypedFirmware `json:",omitempty"`
}

// String returns the String value of the section if it makes sense,
// such as the name or the version string.
func (s *Section) String() string {
	switch s.Header.Type {
	case SectionTypeUserInterface:
		return s.Name
	case SectionTypeVersion:
		return "Version " + s.Version
	}
	return ""
}

// SetType sets the section type in the header and updates the string name.
func (s *Section) SetType(t SectionType) {
	s.Header.Type = t
	s.Type = t.String()
}

// Buf returns the buffer.
// Used mostly for things interacting with the Firmware interface.
func (s *Section) Buf() []byte {
	return s.buf
}

// SetBuf sets the buffer.
// Used mostly for things interacting with the Firmware interface.
func (s *Section) SetBuf(buf []byte) {
	s.buf = buf
}

// Apply calls the visitor on the Section.
func (s *Section) Apply(v Visitor) error {
	return v.Visit(s)
}

// ApplyChildren calls the visitor on each child node of Section.
func (s *Section) ApplyChildren(v Visitor) error {
	for _, f := range s.Encapsulated {
		if err := f.Value.Apply(v); err != nil {
			return err
		}
	}
	return nil
}

// CreateSection creates a new section from minimal components.
// The guid is only used in the case of a GUID Defined section type.
func CreateSection(t SectionType, buf []byte, encap []Firmware, g *guid.GUID) (*Section, error) {
	s := &Section{}

	s.Header.Type = t
	// Map type to string.
	s.Type = s.Header.Type.String()

	s.buf = append([]byte{}, buf...) // Copy out buffer.

	for _, e := range encap {
		s.Encapsulated = append(s.Encapsulated, MakeTyped(e))
	}

	// Create type section header
	switch s.Header.Type {
	case SectionTypeGUIDDefined:
		if g == nil {
			return nil, errors.New("guid was nil, can't make guid defined section")
		}
		guidDefHeader := &SectionGUIDDefined{}
		guidDefHeader.GUID = *g
		switch *g {
		case compression.BROTLIGUID:
			guidDefHeader.Compression = "BROTLI"
		case compression.LZMAGUID:
			guidDefHeader.Compression = "LZMA"
		case compression.LZMAX86GUID:
			guidDefHeader.Compression = "LZMAX86"
		default:
			guidDefHeader.Compression = "UNKNOWN"
		}
		guidDefHeader.Attributes = uint16(GUIDEDSectionProcessingRequired)
		s.TypeSpecific = &TypeSpecificHeader{SectionTypeGUIDDefined, guidDefHeader}
	}

	return s, nil
}

// GenSecHeader generates a full binary header for the section data.
// It assumes that the passed in section struct already contains section data in the buffer,
// the section type in the Type field, and the type specific header in the TypeSpecific field.
// It modifies the calling Section.
func (s *Section) GenSecHeader() error {
	var err error
	// Calculate size
	headerLen := uint32(SectionMinLength)
	if s.TypeSpecific != nil && s.TypeSpecific.Header != nil {
		headerLen += s.TypeSpecific.Header.GetBinHeaderLen()
	}
	s.Header.ExtendedSize = uint32(len(s.buf)) + headerLen // TS header lengths are part of headerLen at this point
	if s.Header.ExtendedSize >= 0xFFFFFF {
		headerLen += 4 // Add space for the extended header.
		s.Header.ExtendedSize += 4
	}

	// Set the correct data offset for GUID Defined headers.
	// This is terrible
	if s.Header.Type == SectionTypeGUIDDefined {
		gd := s.TypeSpecific.Header.(*SectionGUIDDefined)
		gd.DataOffset = uint16(headerLen)
		// append type specific header in front of data
		tsh := new(bytes.Buffer)
		if err = binary.Write(tsh, binary.LittleEndian, &gd.SectionGUIDDefinedHeader); err != nil {
			return err
		}
		s.buf = append(tsh.Bytes(), s.buf...)
	}

	// Append common header
	s.Header.Size = Write3Size(uint64(s.Header.ExtendedSize))
	h := new(bytes.Buffer)
	if s.Header.ExtendedSize >= 0xFFFFFF {
		err = binary.Write(h, binary.LittleEndian, &s.Header)
	} else {
		err = binary.Write(h, binary.LittleEndian, &s.Header.SectionHeader)
	}
	if err != nil {
		return err
	}
	s.buf = append(h.Bytes(), s.buf...)
	return nil
}

// ErrOversizeHdr is the error returned by NewSection when the header is oversize.
type ErrOversizeHdr struct {
	hdrsiz uintptr
	bufsiz int
}

func (e *ErrOversizeHdr) Error() string {
	return fmt.Sprintf("Header size %#x larger than available data %#x", e.hdrsiz, e.bufsiz)
}

// NewSection parses a sequence of bytes and returns a Section
// object, if a valid one is passed, or an error.
func NewSection(buf []byte, fileOrder int) (*Section, error) {
	s := Section{FileOrder: fileOrder}
	// Read in standard header.
	r := bytes.NewReader(buf)
	if err := binary.Read(r, binary.LittleEndian, &s.Header.SectionHeader); err != nil {
		return nil, err
	}

	// Map type to string.
	s.Type = s.Header.Type.String()

	headerSize := unsafe.Sizeof(SectionHeader{})
	switch s.Header.Type {
	case SectionTypeAll, SectionTypeCompression, SectionTypeGUIDDefined, SectionTypeDisposable,
		SectionTypePE32, SectionTypePIC, SectionTypeTE, SectionTypeDXEDepEx, SectionTypeVersion,
		SectionTypeUserInterface, SectionTypeCompatibility16, SectionTypeFirmwareVolumeImage,
		SectionTypeFreeformSubtypeGUID, SectionTypeRaw, SectionTypePEIDepEx, SectionMMDepEx:
		if s.Header.Size == [3]uint8{0xFF, 0xFF, 0xFF} {
			// Extended Header
			if err := binary.Read(r, binary.LittleEndian, &s.Header.ExtendedSize); err != nil {
				return nil, err
			}
			if s.Header.ExtendedSize == 0xFFFFFFFF {
				return nil, errors.New("section size and extended size are all FFs! there should not be free space inside a file")
			}
			headerSize = unsafe.Sizeof(SectionExtHeader{})
		} else {
			// Copy small size into big for easier handling.
			// Section's extended size is 32 bits unlike file's
			s.Header.ExtendedSize = uint32(Read3Size(s.Header.Size))
		}
	default:
		s.Header.ExtendedSize = uint32(Read3Size(s.Header.Size))
		if buflen := len(buf); int(s.Header.ExtendedSize) > buflen {
			s.Header.ExtendedSize = uint32(buflen)
		}
	}

	if buflen := len(buf); int(s.Header.ExtendedSize) > buflen {
		return nil, fmt.Errorf("section size mismatch! Section has size %v, but buffer is %v bytes big",
			s.Header.ExtendedSize, buflen)
	}

	if ReadOnly {
		s.buf = buf[:s.Header.ExtendedSize]
	} else {
		// Copy out the buffer.
		newBuf := buf[:s.Header.ExtendedSize]
		s.buf = make([]byte, s.Header.ExtendedSize)
		copy(s.buf, newBuf)
	}

	// Section type specific data
	switch s.Header.Type {
	case SectionTypeGUIDDefined:
		typeSpec := &SectionGUIDDefined{}
		if err := binary.Read(r, binary.LittleEndian, &typeSpec.SectionGUIDDefinedHeader); err != nil {
			return nil, err
		}
		s.TypeSpecific = &TypeSpecificHeader{Type: SectionTypeGUIDDefined, Header: typeSpec}

		// Determine how to interpret the section based on the GUID.
		var encapBuf []byte
		if typeSpec.Attributes&uint16(GUIDEDSectionProcessingRequired) != 0 && !DisableDecompression {
			if compressor := compression.CompressorFromGUID(&typeSpec.GUID); compressor != nil {
				typeSpec.Compression = compressor.Name()
				var err error
				encapBuf, err = compressor.Decode(buf[typeSpec.DataOffset:])
				if err != nil {
					log.Errorf("%v", err)
					typeSpec.Compression = "UNKNOWN"
					encapBuf = []byte{}
				}
			} else {
				typeSpec.Compression = "UNKNOWN"
			}
		}

		for i, offset := 0, uint64(0); offset < uint64(len(encapBuf)); i++ {
			encapS, err := NewSection(encapBuf[offset:], i)
			if err != nil {
				return nil, fmt.Errorf("error parsing encapsulated section #%d at offset %d: %v",
					i, offset, err)
			}
			// Align to 4 bytes for now. The PI Spec doesn't say what alignment it should be
			// but UEFITool aligns to 4 bytes, and this seems to work on everything I have.
			offset = Align4(offset + uint64(encapS.Header.ExtendedSize))
			s.Encapsulated = append(s.Encapsulated, MakeTyped(encapS))
		}

	case SectionTypeUserInterface:
		if len(s.buf) <= int(headerSize) {
			return nil, &ErrOversizeHdr{hdrsiz: headerSize, bufsiz: len(s.buf)}
		}
		s.Name = unicode.UCS2ToUTF8(s.buf[headerSize:])

	case SectionTypeVersion:
		if len(s.buf) <= int(headerSize+2) {
			return nil, &ErrOversizeHdr{hdrsiz: headerSize + 2, bufsiz: len(s.buf)}
		}
		s.BuildNumber = binary.LittleEndian.Uint16(s.buf[headerSize : headerSize+2])
		s.Version = unicode.UCS2ToUTF8(s.buf[headerSize+2:])

	case SectionTypeFirmwareVolumeImage:
		if len(s.buf) <= int(headerSize) {
			return nil, &ErrOversizeHdr{hdrsiz: headerSize, bufsiz: len(s.buf)}
		}
		fv, err := NewFirmwareVolume(s.buf[headerSize:], 0, true)
		if err != nil {
			return nil, err
		}
		s.Encapsulated = []*TypedFirmware{MakeTyped(fv)}

	case SectionTypeDXEDepEx, SectionTypePEIDepEx, SectionMMDepEx:
		if len(s.buf) <= int(headerSize) {
			return nil, &ErrOversizeHdr{hdrsiz: headerSize, bufsiz: len(s.buf)}
		}
		var err error
		if s.DepEx, err = parseDepEx(s.buf[headerSize:]); err != nil {
			log.Warnf("%v", err)
		}
	}

	return &s, nil
}

func parseDepEx(b []byte) ([]DepExOp, error) {
	depEx := []DepExOp{}
	r := bytes.NewBuffer(b)
	for {
		opCodeByte, err := r.ReadByte()
		if err != nil {
			return nil, errors.New("invalid DEPEX, no END")
		}
		if opCodeStr, ok := DepExOpCodes[opCodeByte]; ok {
			op := DepExOp{OpCode: opCodeStr}
			if opCodeStr == "BEFORE" || opCodeStr == "AFTER" || opCodeStr == "PUSH" {
				op.GUID = &guid.GUID{}
				if err := binary.Read(r, binary.LittleEndian, op.GUID); err != nil {
					return nil, fmt.Errorf("invalid DEPEX, could not read GUID: %v", err)
				}
			}
			depEx = append(depEx, op)
			if opCodeStr == "END" {
				break
			}
		} else {
			return nil, fmt.Errorf("invalid DEPEX opcode, %#v", opCodeByte)
		}
	}
	return depEx, nil
}
