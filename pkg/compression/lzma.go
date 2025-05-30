package compression

import (
	"bytes"
	"io"

	"github.com/ulikunitz/xz/lzma"
)

// Mapping from compression level to dictionary size.
var lzmaDictCapExps = []uint{18, 20, 21, 22, 22, 23, 23, 24, 25, 26}
var compressionLevel = 7

// LZMA implements Compressor and uses a Go-based implementation.
type LZMA struct{}

// Name returns the type of compression employed.
func (c *LZMA) Name() string {
	return "LZMA"
}

// Decode decodes a byte slice of LZMA data.
func (c *LZMA) Decode(encodedData []byte) ([]byte, error) {
	r, err := lzma.NewReader(bytes.NewBuffer(encodedData))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

// Encode encodes a byte slice with LZMA.
func (c *LZMA) Encode(decodedData []byte) ([]byte, error) {
	// These options are supported by the xz's LZMA command and EDK2's LZMA.
	wc := lzma.WriterConfig{
		SizeInHeader: true,
		Size:         int64(len(decodedData)),
		EOSMarker:    false,
		Properties:   &lzma.Properties{LC: 3, LP: 0, PB: 2},
		DictCap:      1 << lzmaDictCapExps[compressionLevel],
	}
	if err := wc.Verify(); err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	w, err := wc.NewWriter(buf)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(w, bytes.NewBuffer(decodedData)); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
