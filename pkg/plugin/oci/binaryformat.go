package oci

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"errors"
	"fmt"
	"io"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	peExecutableImage = 0x0002
	peDLL             = 0x2000
)

var (
	elfSystems   = map[string]bool{"linux": true, "freebsd": true, "netbsd": true, "openbsd": true, "dragonfly": true, "solaris": true, "illumos": true}
	elfMachines  = map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64, "386": elf.EM_386, "arm": elf.EM_ARM, "ppc64": elf.EM_PPC64, "ppc64le": elf.EM_PPC64, "s390x": elf.EM_S390, "riscv64": elf.EM_RISCV, "loong64": elf.EM_LOONGARCH}
	machoCPUs    = map[string]macho.Cpu{"amd64": macho.CpuAmd64, "arm64": macho.CpuArm64, "386": macho.Cpu386, "arm": macho.CpuArm}
	peMachines   = map[string]uint16{"amd64": pe.IMAGE_FILE_MACHINE_AMD64, "arm64": pe.IMAGE_FILE_MACHINE_ARM64, "386": pe.IMAGE_FILE_MACHINE_I386, "arm": pe.IMAGE_FILE_MACHINE_ARMNT}
	errNotNative = errors.New("not a native executable")
)

// checkExecutableFormat proves that the bytes are a native executable for the
// platform's operating system and architecture. It only parses headers; the file
// is never executed, and the contents are not scanned for secrets.
func checkExecutableFormat(reader io.ReaderAt, platform *v1.Platform) error {
	var err error
	switch {
	case elfSystems[platform.OS]:
		err = checkELF(reader, platform)
	case platform.OS == "darwin":
		err = checkMachO(reader, platform)
	case platform.OS == "windows":
		err = checkPE(reader, platform)
	default:
		err = fmt.Errorf("%w: operating system %q has no supported executable format", errNotNative, platform.OS)
	}
	if err == nil {
		return nil
	}
	return errUtils.Build(fmt.Errorf("%w for %s: %w", errUtils.ErrPluginBundleExecutableInvalid, platform.String(), err)).
		WithHintf("Build the plugin for %s (for example with `atmos plugin build`) and point `spec.platforms.%s.executable` at the compiled binary.", platform.String(), platform.String()).
		Err()
}

func checkELF(reader io.ReaderAt, platform *v1.Platform) error {
	file, err := elf.NewFile(reader)
	if err != nil {
		return fmt.Errorf("%w: expected an ELF executable: %w", errNotNative, err)
	}
	defer file.Close()
	machine, supported := elfMachines[platform.Architecture]
	if !supported {
		return fmt.Errorf("%w: architecture %q has no supported ELF machine", errNotNative, platform.Architecture)
	}
	if file.Type != elf.ET_EXEC && file.Type != elf.ET_DYN {
		return fmt.Errorf("%w: ELF file type %s is not executable", errNotNative, file.Type)
	}
	if file.Machine != machine {
		return fmt.Errorf("%w: ELF machine is %s, expected %s", errNotNative, file.Machine, machine)
	}
	if platform.Architecture == "ppc64" && file.Data != elf.ELFDATA2MSB || platform.Architecture == "ppc64le" && file.Data != elf.ELFDATA2LSB {
		return fmt.Errorf("%w: ELF byte order does not match %s", errNotNative, platform.Architecture)
	}
	return nil
}

func checkMachO(reader io.ReaderAt, platform *v1.Platform) error {
	cpu, supported := machoCPUs[platform.Architecture]
	if !supported {
		return fmt.Errorf("%w: architecture %q has no supported Mach-O CPU", errNotNative, platform.Architecture)
	}
	if file, err := macho.NewFile(reader); err == nil {
		defer file.Close()
		return matchMachO(file, cpu)
	}
	fat, err := macho.NewFatFile(reader)
	if err != nil {
		return fmt.Errorf("%w: expected a Mach-O or universal executable", errNotNative)
	}
	defer fat.Close()
	for index := range fat.Arches {
		if matchMachO(fat.Arches[index].File, cpu) == nil {
			return nil
		}
	}
	return fmt.Errorf("%w: universal binary has no executable slice for %s", errNotNative, platform.Architecture)
}

func matchMachO(file *macho.File, cpu macho.Cpu) error {
	if file.Type != macho.TypeExec {
		return fmt.Errorf("%w: Mach-O file type %s is not executable", errNotNative, file.Type)
	}
	if file.Cpu != cpu {
		return fmt.Errorf("%w: Mach-O CPU is %s, expected %s", errNotNative, file.Cpu, cpu)
	}
	return nil
}

func checkPE(reader io.ReaderAt, platform *v1.Platform) error {
	file, err := pe.NewFile(reader)
	if err != nil {
		return fmt.Errorf("%w: expected a PE executable: %w", errNotNative, err)
	}
	defer file.Close()
	machine, supported := peMachines[platform.Architecture]
	if !supported {
		return fmt.Errorf("%w: architecture %q has no supported PE machine", errNotNative, platform.Architecture)
	}
	if file.Machine != machine {
		return fmt.Errorf("%w: PE machine is %#x, expected %#x", errNotNative, file.Machine, machine)
	}
	if file.Characteristics&peExecutableImage == 0 || file.Characteristics&peDLL != 0 {
		return fmt.Errorf("%w: PE image is not an executable program", errNotNative)
	}
	return nil
}
