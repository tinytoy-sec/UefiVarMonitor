@echo off
qemu-system-x86_64.exe ^
  -nodefaults ^
  -vga std ^
  -machine q35 ^
  -m 128M ^
  -drive if=pflash,format=raw,readonly,file=OVMF_CODE.fd ^
  -drive if=pflash,format=raw,file=OVMF_VARS.fd ^
  -drive format=raw,file=fat:rw:.\target\x86_64-unknown-uefi ^
  -serial mon:stdio

:: The former puts serial output to the console of the host.
:: The latter opens the port for telnet.
::  -serial mon:stdio
::  -serial telnet:localhost:12345,server,nowait
