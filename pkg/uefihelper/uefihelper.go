package uefihelper

import (
	"errors"
	"os"

	"github.com/tinytoy-sec/UefiVarMonitor/pkg/uefi"
	"github.com/tinytoy-sec/UefiVarMonitor/pkg/visitors"
)

func Run(args ...string) error {
	if len(args) == 0 {
		return errors.New("at least one argument is required")
	}

	v, err := visitors.ParseCLI(args[1:])
	if err != nil {
		return err
	}

	path := args[0]
	f, err := os.Stat(path)
	if err != nil {
		return err
	}
	var parsedRoot uefi.Firmware
	if m := f.Mode(); m.IsDir() {
		pd := visitors.ParseDir{BasePath: path}
		if parsedRoot, err = pd.Parse(); err != nil {
			return err
		}
		a := visitors.Assemble{}
		if err = a.Run(parsedRoot); err != nil {
			return err
		}
	} else {
		image, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parsedRoot, err = uefi.Parse(image)
		if err != nil {
			return err
		}
	}

	return visitors.ExecuteCLI(parsedRoot, v)
}
