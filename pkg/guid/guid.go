package guid

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinytoy-sec/UefiVarMonitor/pkg/log"
)

const (
	// Size 表示GUID的字节数
	Size = 16
	// UExample 是字符串GUID的示例
	UExample  = "01234567-89AB-CDEF-0123-456789ABCDEF"
	strFormat = "%02X%02X%02X%02X-%02X%02X-%02X%02X-%02X%02X-%02X%02X%02X%02X%02X%02X"
)

var (
	fields = [...]int{4, 2, 2, 1, 1, 1, 1, 1, 1, 1, 1}
)

// GUID 表示唯一标识符
type GUID [Size]byte

func reverse(b []byte) {
	for i := 0; i < len(b)/2; i++ {
		other := len(b) - i - 1
		b[other], b[i] = b[i], b[other]
	}
}

// Parse 解析GUID字符串
func Parse(s string) (*GUID, error) {
	// 移除所有连字符以便更容易解析
	stripped := strings.Replace(s, "-", "", -1)
	decoded, err := hex.DecodeString(stripped)
	if err != nil {
		return nil, fmt.Errorf("GUID字符串格式不正确，需要格式为\n%v\n，得到\n%v",
			UExample, s)
	}

	if len(decoded) != Size {
		return nil, fmt.Errorf("GUID字符串长度不正确，需要格式为\n%v\n，得到\n%v",
			UExample, s)
	}

	u := GUID{}
	i := 0
	copy(u[:], decoded[:])
	// 修正字节序
	for _, fieldlen := range fields {
		reverse(u[i : i+fieldlen])
		i += fieldlen
	}
	return &u, nil
}

// MustParse 解析GUID字符串或引发恐慌
func MustParse(s string) *GUID {
	guid, err := Parse(s)
	if err != nil {
		log.Fatalf("%v", err)
	}
	return guid
}

func (u GUID) String() string {
	// 非指针接收器，所以不需要手动复制
	i := 0
	// 反转字节序
	for _, fieldlen := range fields {
		reverse(u[i : i+fieldlen])
		i += fieldlen
	}
	// 转换为[]interface{}以便于打印
	b := make([]interface{}, Size)
	for i := range u[:] {
		b[i] = u[i]
	}
	return fmt.Sprintf(strFormat, b...)
}

// MarshalJSON 实现marshaller接口
// 这允许我们实际读取和编辑json文件
func (u *GUID) MarshalJSON() ([]byte, error) {
	return []byte(`{"GUID" : "` + u.String() + `"}`), nil
}

// UnmarshalJSON 实现unmarshaller接口
// 这允许我们实际读取和编辑json文件
func (u *GUID) UnmarshalJSON(b []byte) error {
	j := make(map[string]string)

	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	g, err := Parse(j["GUID"])
	if err != nil {
		return err
	}
	copy(u[:], g[:])
	return nil
}
