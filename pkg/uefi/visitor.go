package uefi

// 访问者接口定义了遍历固件结构的方法
type Visitor interface {
	Run(Firmware) error
	Visit(Firmware) error
}
