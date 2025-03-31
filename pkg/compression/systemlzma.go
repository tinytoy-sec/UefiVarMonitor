package compression

import (
	"bytes"
	"encoding/binary"
	"os/exec"
)

// SystemLZMA implements Compression and calls out to the system's compressor
// (except for Decode which uses the Go-based decompressor). The sytem's
// compressor is typically faster and generates smaller files than the Go-based
// implementation.
type SystemLZMA struct {
	xzPath string
}

// Name returns the type of compression employed.
func (c *SystemLZMA) Name() string {
	return "LZMA"
}

// Decode decodes a byte slice of LZMA data.
func (c *SystemLZMA) Decode(encodedData []byte) ([]byte, error) {

	return (&LZMA{}).Decode(encodedData)
}

// Encode encodes a byte slice with LZMA.
func (c *SystemLZMA) Encode(decodedData []byte) ([]byte, error) {
	cmd := exec.Command(c.xzPath, "--format=lzma", "-7", "--stdout")
	cmd.Stdin = bytes.NewBuffer(decodedData)
	encodedData, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	buf := &bytes.Buffer{}
	if err := binary.Write(buf, binary.LittleEndian, uint64(len(decodedData))); err != nil {
		return nil, err
	}
	copy(encodedData[5:5+8], buf.Bytes())
	return encodedData, nil
}
