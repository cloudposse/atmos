// Package ocitest builds tiny, never-executed native executables for plugin packaging tests.
//
// The images contain only a valid ELF, Mach-O, or PE header for the requested
// platform. They satisfy the pack-time format check without running any code.
package ocitest

import (
	"bytes"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"fmt"
)

// Binary returns a minimal, non-runnable executable image for goos/goarch.
// It panics for platforms it cannot represent so a bad test fixture fails loudly.
//
//nolint:lintroller // Test support with no I/O.
func Binary(goos, goarch string) []byte {
	switch goos {
	case "darwin":
		return machO(goarch)
	case "windows":
		return portableExecutable(goarch)
	case "linux", "freebsd", "netbsd", "openbsd", "dragonfly", "solaris", "illumos":
		return elfExecutable(goarch)
	default:
		panic(fmt.Sprintf("ocitest: unsupported os %q", goos))
	}
}

const (
	elfHeader64Size = 64
	elfHeader32Size = 52
	macho64Bit      = uint32(macho.CpuAmd64) & ^uint32(macho.Cpu386)
	peHeaderOffset  = 128
)

type elfMachine struct {
	machine uint16
	class   byte
	order   binary.ByteOrder
}

var elfMachines = map[string]elfMachine{
	"amd64":   {62, 2, binary.LittleEndian},
	"arm64":   {183, 2, binary.LittleEndian},
	"386":     {3, 1, binary.LittleEndian},
	"arm":     {40, 1, binary.LittleEndian},
	"ppc64le": {21, 2, binary.LittleEndian},
	"ppc64":   {21, 2, binary.BigEndian},
	"s390x":   {22, 2, binary.BigEndian},
	"riscv64": {243, 2, binary.LittleEndian},
	"loong64": {258, 2, binary.LittleEndian},
}

func elfExecutable(goarch string) []byte {
	target, found := elfMachines[goarch]
	if !found {
		panic(fmt.Sprintf("ocitest: unsupported architecture %q", goarch))
	}
	var buffer bytes.Buffer
	data := byte(1)
	if target.order == binary.BigEndian {
		data = 2
	}
	buffer.Write([]byte{0x7f, 'E', 'L', 'F', target.class, data, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	write := func(value any) { _ = binary.Write(&buffer, target.order, value) }
	write(uint16(2)) // ET_EXEC.
	write(target.machine)
	write(uint32(1))
	if target.class == 2 {
		write(uint64(0)) // Entry.
		write(uint64(0)) // Program header offset.
		write(uint64(0)) // Section header offset.
	} else {
		write(uint32(0))
		write(uint32(0))
		write(uint32(0))
	}
	write(uint32(0)) // Flags.
	if target.class == 2 {
		write(uint16(elfHeader64Size))
	} else {
		write(uint16(elfHeader32Size))
	}
	write(uint16(0)) // Program header entry size.
	write(uint16(0)) // Program header count.
	write(uint16(0)) // Section header entry size.
	write(uint16(0)) // Section header count.
	write(uint16(0)) // Section name string table index.
	return buffer.Bytes()
}

var machOCPU = map[string]uint32{"amd64": 0x01000007, "arm64": 0x0100000c, "386": 7, "arm": 12}

func machO(goarch string) []byte {
	cpu, found := machOCPU[goarch]
	if !found {
		panic(fmt.Sprintf("ocitest: unsupported architecture %q", goarch))
	}
	var buffer bytes.Buffer
	magic := macho.Magic32
	if cpu&macho64Bit != 0 {
		magic = macho.Magic64
	}
	for _, value := range []uint32{magic, cpu, 0, 2, 0, 0, 0} { // Magic, CPU, subtype, MH_EXECUTE, ncmds, sizeofcmds, flags.
		_ = binary.Write(&buffer, binary.LittleEndian, value)
	}
	if magic == macho.Magic64 {
		_ = binary.Write(&buffer, binary.LittleEndian, uint32(0)) // Reserved.
	}
	return buffer.Bytes()
}

var peMachines = map[string]uint16{"amd64": 0x8664, "arm64": 0xaa64, "386": 0x14c, "arm": 0x1c4}

func portableExecutable(goarch string) []byte {
	machine, found := peMachines[goarch]
	if !found {
		panic(fmt.Sprintf("ocitest: unsupported architecture %q", goarch))
	}
	header := make([]byte, peHeaderOffset)
	copy(header, "MZ")
	binary.LittleEndian.PutUint32(header[0x3c:], peHeaderOffset)
	var buffer bytes.Buffer
	buffer.Write(header)
	buffer.WriteString("PE\x00\x00")
	for _, value := range []any{machine, uint16(0), uint32(0), uint32(0), uint32(0), uint16(0), uint16(pe.IMAGE_FILE_EXECUTABLE_IMAGE | pe.IMAGE_FILE_LARGE_ADDRESS_AWARE)} {
		_ = binary.Write(&buffer, binary.LittleEndian, value)
	}
	return buffer.Bytes()
}
