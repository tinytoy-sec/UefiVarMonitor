package unicode

import (
	"github.com/tinytoy-sec/UefiVarMonitor/pkg/log"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// 从UCS2转换为UTF8
func UCS2ToUTF8(input []byte) string {
	e := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	output, _, err := transform.Bytes(e.NewDecoder(), input)
	if err != nil {
		log.Errorf("无法解码UCS2: %v", err)
		return string(input)
	}
	// 如果存在，移除空终止符
	if output[len(output)-1] == 0 {
		output = output[:len(output)-1]
	}
	return string(output)
}

// 从UTF8转换为UCS2
func UTF8ToUCS2(input string) []byte {
	e := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	input = input + "\000" // 空终止符
	output, _, err := transform.Bytes(e.NewEncoder(), []byte(input))
	if err != nil {
		log.Errorf("无法编码UCS2: %v", err)
		return []byte(input)
	}
	return output
}
