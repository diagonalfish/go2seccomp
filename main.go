package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
)

// TODO add a verbose flag and do a proper verbose mode
var verbose = false

// need to save the previous instructions to go back and look for the syscall ID
// have found MOVs to 0(SP) as far as 10 instructions behind, so 15 seems like a safe number
const previousInstructionsBufferSize = 15

// wrapper for each findSyscallID by arch
func findSyscallID(arch specs.Arch, previouInstructions []string, curPos int) (int64, error) {
	var i int64
	var err error

	switch arch {
	case specs.ArchX86_64:
		i, err = findSyscallIDx86_64(previouInstructions, curPos)
	case specs.ArchX86:
		i, err = findSyscallIDx86(previouInstructions, curPos)
	case specs.ArchARM:
		i, err = findSyscallIDARM(previouInstructions, curPos)
	default:
		log.Fatalln(arch, "is not supported")
	}

	return i, err
}

func findRuntimeSyscallID(arch specs.Arch, previouInstructions []string, curPos int) (int64, error) {
	var i int64
	var err error

	switch arch {
	case specs.ArchX86_64:
		i, err = findRuntimeSyscallIDx86_64(previouInstructions, curPos)
	case specs.ArchX86:
		i, err = findRuntimeSyscallIDx86_64(previouInstructions, curPos) // Same as x86_64 ?
	case specs.ArchARM:
		i, err = findRuntimeSyscallIDARM(previouInstructions, curPos)
	default:
		log.Fatalln(arch, "is not supported")
	}

	return i, err
}

func findRuntimeSyscallIDx86_64(previouInstructions []string, curPos int) (int64, error) {
	i := 0

	for i < previousInstructionsBufferSize {
		instruction := previouInstructions[curPos%previousInstructionsBufferSize]
		isMOV := strings.Index(instruction, "MOV") != -1
		isAXRegister := strings.Index(instruction, ", AX") != -1

		// runtime·read on syscall/sys_linux_amd64.s has the following for calling the read syscall:
		// MOVL $0, AX
		// SYSCALL
		// However, some compiler optmization changes it to:
		// XORL AX, AX
		// which must be faster to zero the register than using a MOV, so we need to account for this
		isRead := strings.Index(instruction, "XOR") != -1 && strings.Index(instruction, " AX, AX") != -1
		if isRead {
			return 0, nil
		}

		if isMOV && isAXRegister {
			syscallIDBeginning := strings.Index(instruction, "$")
			if syscallIDBeginning == -1 {
				return -1, fmt.Errorf("Failed to find syscall ID on line: %v", instruction)
			}
			syscallIDEnd := strings.Index(instruction, ", AX")

			hex := instruction[syscallIDBeginning+1 : syscallIDEnd]
			id, err := strconv.ParseInt(hex, 0, 64)

			if err != nil {
				return -1, fmt.Errorf("Error parsing hex id: %v", err)
			}
			return id, nil
		}
		i++
		curPos--
	}
	return -1, fmt.Errorf("Failed to find syscall ID")
}

func findRuntimeSyscallIDARM(previouInstructions []string, curPos int) (int64, error) {
	i := 0

	for i < previousInstructionsBufferSize {
		instruction := previouInstructions[curPos%previousInstructionsBufferSize]
		isR7 := strings.Index(instruction, ", R7") != -1
		isNotReg := strings.Index(instruction, "),") == -1 // get the "(R15)," ending in MOVW 0x2c(R15), R7

		if isR7 && isNotReg {
			syscallIDBeginning := strings.Index(instruction, "$")
			if syscallIDBeginning == -1 {
				return -1, fmt.Errorf("Failed to find syscall ID on line: %v", instruction)
			}
			syscallIDEnd := strings.Index(instruction, ", R7")

			hex := instruction[syscallIDBeginning+1 : syscallIDEnd]
			id, err := strconv.ParseInt(hex, 0, 64)

			if err != nil {
				return -1, fmt.Errorf("Error parsing hex id: %v", err)
			}
			return id, nil
		}
		i++
		curPos--
	}
	return -1, fmt.Errorf("Failed to find syscall ID")
}

// findSyscallIDx86_64 goes back from the call until it finds an instruction with the format
// MOVQ $ID, 0(SP), which is the one that pushes the syscall ID onto the base address
// at the SP register
func findSyscallIDx86_64(previouInstructions []string, curPos int) (int64, error) {
	i := 0

	for i < previousInstructionsBufferSize {
		instruction := previouInstructions[curPos%previousInstructionsBufferSize]

		isMOVQ := strings.Index(instruction, "MOVQ") != -1
		isBaseSPAddress := strings.Index(instruction, ", 0(SP)") != -1

		if isMOVQ && isBaseSPAddress {
			syscallIDBeginning := strings.Index(instruction, "$")
			if syscallIDBeginning == -1 {
				return -1, fmt.Errorf("Failed to find syscall ID on line: %v", instruction)
			}
			syscallIDEnd := strings.Index(instruction, ", 0(SP)")

			hex := instruction[syscallIDBeginning+1 : syscallIDEnd]
			id, err := strconv.ParseInt(hex, 0, 64)

			if err != nil {
				return -1, fmt.Errorf("Error parsing hex id: %v", err)
			}
			return id, nil
		}
		i++
		curPos--
	}
	return -1, fmt.Errorf("Failed to find syscall ID")
}

// findSyscallIDx86 goes back from the call until it finds an instruction with the format
// MOVL $ID, 0(SP), which is the one that pushes the syscall ID onto the base address
// at the SP register
func findSyscallIDx86(previouInstructions []string, curPos int) (int64, error) {
	i := 0
	for i < previousInstructionsBufferSize {
		instruction := previouInstructions[curPos%previousInstructionsBufferSize]

		isMOVL := strings.Index(instruction, "MOVL") != -1
		isBaseSPAddress := strings.Index(instruction, ", 0(SP)") != -1

		if isMOVL && isBaseSPAddress {
			syscallIDBeginning := strings.Index(instruction, "$")
			if syscallIDBeginning == -1 {
				return -1, fmt.Errorf("Failed to find syscall ID on line: %v", instruction)
			}
			syscallIDEnd := strings.Index(instruction, ", 0(SP)")

			hex := instruction[syscallIDBeginning+1 : syscallIDEnd]
			id, err := strconv.ParseInt(hex, 0, 64)

			if err != nil {
				return -1, fmt.Errorf("Error parsing hex id: %v", err)
			}
			return id, nil
		}
		i++
		curPos--
	}
	return -1, fmt.Errorf("Failed to find syscall ID")
}

func findSyscallIDARM(previouInstructions []string, curPos int) (int64, error) {
	i := 0

	for i < previousInstructionsBufferSize {
		instruction := previouInstructions[curPos%previousInstructionsBufferSize]

		isMOVW := strings.Index(instruction, "MOVW") != -1
		isBaseSPAddress := strings.Index(instruction, ", R0") != -1
		syscallIDBeginning := strings.Index(instruction, "$")

		if isMOVW && isBaseSPAddress && (syscallIDBeginning != -1) {
			syscallIDEnd := strings.Index(instruction, ", R0")

			hex := instruction[syscallIDBeginning+1 : syscallIDEnd]
			id, err := strconv.ParseInt(hex, 0, 64)

			if err != nil {
				return -1, fmt.Errorf("Error parsing hex id: %v", err)
			}
			return id, nil
		}
		i++
		curPos--
	}
	return -1, fmt.Errorf("Failed to find syscall ID")
}

func getSyscallList(disassambled *os.File, arch specs.Arch) []string {

	scanner := bufio.NewScanner(disassambled)

	// keep a few of the past instructions in a buffer so we can look back and find the syscall ID
	previousInstructions := make([]string, previousInstructionsBufferSize)
	lineCount := 0
	syscalls := getDefaultSyscalls(arch)

	fmt.Println("Scanning disassembled binary for syscall IDs")

	currentFunction := ""
	for scanner.Scan() {
		instruction := scanner.Text()
		previousInstructions[lineCount%previousInstructionsBufferSize] = instruction

		if len(instruction) > 5 && instruction[0:4] == "TEXT" {
			currentFunction = parseFunctionName(instruction)
		}

		// function call to one of the 5 functions from the syscall package
		if isSyscallPkgCall(arch, instruction) {
			id, err := findSyscallID(arch, previousInstructions, lineCount)
			if err != nil {
				log.Printf("Failed to find syscall ID for line %v: %v, reason: %v\n", lineCount+1, instruction, err)
				lineCount++
				continue
			}
			syscalls[id] = true
		}
		// the runtime package doesn't use the functions on the syscall package, instead it uses SYSCALL directly
		if isRuntimeSyscall(arch, instruction, currentFunction) {
			id, err := findRuntimeSyscallID(arch, previousInstructions, lineCount)
			if err != nil {
				log.Printf("Failed to find syscall ID for line %v: \n\t%v\n\treason: %v\n", lineCount+1, instruction, err)
				lineCount++
				continue
			}
			syscalls[id] = true
		}
		lineCount++
	}

	syscallsList := make([]string, len(syscalls))
	i := 0

	for id := range syscalls {
		name, ok := syscallIDtoName[arch][id]
		if !ok {
			fmt.Printf("Sycall ID %v not available on the ID->name map\n", id)
		} else {
			syscallsList[i] = name
			i++
		}
	}

	sort.Strings(syscallsList)

	return syscallsList
}

func main() {
	flag.Parse()

	if len(flag.Args()) < 2 {
		fmt.Println("Usage: go2seccomp /path/to/binary /path/to/profile.json")
		os.Exit(1)
	}

	binaryPath := flag.Args()[0]
	profilePath := flag.Args()[1]

	f := openElf(binaryPath)

	if !isGoBinary(f) {
		fmt.Println(binaryPath, "doesn't seems to be a Go binary")
		os.Exit(1)
	}

	arch := getArch(f)

	disassambled := disassamble(binaryPath)
	defer disassambled.Close()
	defer os.Remove("disassembled.asm")

	syscallsList := getSyscallList(disassambled, arch)

	fmt.Printf("Syscalls detected (total: %v): %v\n", len(syscallsList), syscallsList)

	writeProfile(syscallsList, arch, profilePath)
}
