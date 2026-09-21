package builder

import (
	"debug/elf"
	"debug/macho"
	"errors"
	"fmt"
	"sort"
	"strings"

	"tinygo.org/x/go-llvm"
)

type callerLine struct {
	offset int64
	length uint64
	file   string
	line   uint64
}

type callerFunction struct {
	name string
	fn   llvm.Value
}

func orderCallerFunctions(path string, functions []callerFunction) ([]callerFunction, error) {
	file, err := elf.Open(path)
	addresses := make(map[string]uint64)
	if err == nil {
		defer file.Close()
		symbols, err := file.Symbols()
		if err != nil {
			return nil, err
		}
		for _, symbol := range symbols {
			if elf.ST_TYPE(symbol.Info) != elf.STT_FUNC || symbol.Value == 0 {
				continue
			}
			addCallerSymbol(addresses, symbol.Name, symbol.Value)
		}
	} else {
		file, err := macho.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		for _, symbol := range file.Symtab.Syms {
			if symbol.Value == 0 {
				continue
			}
			addCallerSymbol(addresses, strings.TrimPrefix(symbol.Name, "_"), symbol.Value)
		}
	}

	ordered := append([]callerFunction(nil), functions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, leftOK := addresses[ordered[i].fn.Name()]
		right, rightOK := addresses[ordered[j].fn.Name()]
		if leftOK != rightOK {
			return leftOK
		}
		if left != right {
			return left < right
		}
		return ordered[i].name < ordered[j].name
	})
	return ordered, nil
}

func addCallerSymbol(addresses map[string]uint64, name string, address uint64) {
	addresses[name] = address
	if index := strings.LastIndex(name, ".llvm."); index >= 0 {
		name = name[:index]
		if _, ok := addresses[name]; !ok {
			addresses[name] = address
		}
	}
}

func setCallerFunctions(mod llvm.Module, machine llvm.TargetMachine, functions []callerFunction, generation int) {
	tableGlobal := mod.NamedGlobal("runtime.funcTable")
	lengthGlobal := mod.NamedGlobal("runtime.funcTableLen")
	sortedGlobal := mod.NamedGlobal("runtime.funcTableSorted")
	if tableGlobal.IsNil() || lengthGlobal.IsNil() || sortedGlobal.IsNil() {
		return
	}

	ctx := mod.Context()
	targetData := machine.CreateTargetData()
	defer targetData.Dispose()
	uintptrType := ctx.IntType(targetData.PointerSize() * 8)
	ptrType := llvm.PointerType(ctx.Int8Type(), 0)
	stringType := mod.GetTypeByName("runtime._string")
	funcType := mod.GetTypeByName("runtime.Func")

	entries := make([]llvm.Value, 0, len(functions))
	for i, function := range functions {
		nameData := ctx.ConstString(function.name, false)
		nameGlobal := llvm.AddGlobal(mod, nameData.Type(), fmt.Sprintf("runtime.funcName.%d.%d", generation, i))
		nameGlobal.SetInitializer(nameData)
		nameGlobal.SetGlobalConstant(true)
		nameGlobal.SetLinkage(llvm.PrivateLinkage)
		nameGlobal.SetUnnamedAddr(true)
		nameGlobal.SetAlignment(1)
		namePointer := llvm.ConstGEP(nameData.Type(), nameGlobal, []llvm.Value{
			llvm.ConstInt(ctx.Int32Type(), 0, false),
			llvm.ConstInt(ctx.Int32Type(), 0, false),
		})
		name := llvm.ConstNamedStruct(stringType, []llvm.Value{
			llvm.ConstPointerCast(namePointer, ptrType),
			llvm.ConstInt(uintptrType, uint64(len(function.name)), false),
		})
		entries = append(entries, llvm.ConstNamedStruct(funcType, []llvm.Value{
			llvm.ConstPtrToInt(function.fn, uintptrType),
			name,
		}))
	}

	lengthGlobal.SetInitializer(llvm.ConstInt(uintptrType, uint64(len(entries)), false))
	sortedGlobal.SetInitializer(llvm.ConstInt(sortedGlobal.GlobalValueType(), 1, false))
	arrayType := llvm.ArrayType(funcType, len(entries))
	array := llvm.AddGlobal(mod, arrayType, fmt.Sprintf("runtime.funcTable.data.%d", generation))
	array.SetInitializer(llvm.ConstArray(funcType, entries))
	array.SetGlobalConstant(true)
	array.SetLinkage(llvm.InternalLinkage)
	tableGlobal.SetInitializer(llvm.ConstGEP(arrayType, array, []llvm.Value{
		llvm.ConstInt(ctx.Int32Type(), 0, false),
		llvm.ConstInt(ctx.Int32Type(), 0, false),
	}))
}

func callerFunctionsEqual(left, right []callerFunction) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].fn.Name() != right[i].fn.Name() {
			return false
		}
	}
	return true
}

func readCallerLines(path, anchorName string) ([]callerLine, error) {
	file, err := elf.Open(path)
	var anchor uint64
	var addresses []addressLine
	if err == nil {
		defer file.Close()
		symbols, err := file.Symbols()
		if err != nil {
			return nil, err
		}
		for _, symbol := range symbols {
			if symbol.Name == anchorName {
				anchor = symbol.Value
				break
			}
		}
		data, err := file.DWARF()
		if err != nil {
			return nil, err
		}
		addresses, err = readProgramSizeFromDWARF(data, 0, 0, true)
		if err != nil {
			return nil, err
		}
	} else {
		anchor, addresses, err = readMachOCallerLines(path, anchorName)
		if err != nil {
			return nil, err
		}
	}
	if anchor == 0 {
		return nil, fmt.Errorf("could not find caller metadata anchor %q", anchorName)
	}
	sort.Slice(addresses, func(i, j int) bool {
		return addresses[i].Address < addresses[j].Address
	})

	lines := make([]callerLine, 0, len(addresses))
	for _, address := range addresses {
		if address.IsVariable || address.Length == 0 || address.File == "" || address.Line == 0 {
			continue
		}
		line := callerLine{
			offset: int64(address.Address) - int64(anchor),
			length: address.Length,
			file:   address.File,
			line:   address.Line,
		}
		if len(lines) != 0 {
			previous := &lines[len(lines)-1]
			if previous.offset+int64(previous.length) == line.offset &&
				previous.file == line.file &&
				previous.line == line.line {
				previous.length += line.length
				continue
			}
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func readMachOCallerLines(path, anchorName string) (uint64, []addressLine, error) {
	file, err := macho.Open(path)
	if err != nil {
		return 0, nil, err
	}
	defer file.Close()

	var anchor uint64
	for _, symbol := range file.Symtab.Syms {
		if strings.TrimPrefix(symbol.Name, "_") == anchorName {
			anchor = symbol.Value
			break
		}
	}

	const (
		nFun = 0x24
		nOSO = 0x66
	)
	var lines []addressLine
	var objectSymbols map[string]int
	var objectLines []addressLine
	var previous macho.Symbol
	for _, symbol := range file.Symtab.Syms {
		switch {
		case symbol.Type == nOSO:
			objectSymbols, objectLines, err = readMachOSymbolAddresses(symbol.Name)
			if err != nil {
				objectSymbols = nil
				objectLines = nil
			}
		case symbol.Type == nFun && symbol.Name == "" && previous.Type == nFun && previous.Name != "":
			address := previous.Value
			length := symbol.Value
			index, ok := objectSymbols[previous.Name]
			for ok && index >= 0 && length != 0 && index < len(objectLines) {
				line := objectLines[index]
				line.Address = address
				if line.Length > length {
					break
				}
				lines = append(lines, line)
				index++
				length -= line.Length
				address += line.Length
			}
		}
		previous = symbol
	}
	return anchor, lines, nil
}

func setCallerLines(mod llvm.Module, machine llvm.TargetMachine, lines []callerLine, generation int) error {
	tableGlobal := mod.NamedGlobal("runtime.lineTable")
	lengthGlobal := mod.NamedGlobal("runtime.lineTableLen")
	filesGlobal := mod.NamedGlobal("runtime.lineFiles")
	filesLengthGlobal := mod.NamedGlobal("runtime.lineFilesLen")
	if tableGlobal.IsNil() || lengthGlobal.IsNil() {
		return nil
	}

	ctx := mod.Context()
	targetData := machine.CreateTargetData()
	defer targetData.Dispose()
	uintptrType := ctx.IntType(targetData.PointerSize() * 8)
	ptrType := llvm.PointerType(ctx.Int8Type(), 0)
	stringType := mod.GetTypeByName("runtime._string")
	lineType := mod.GetTypeByName("runtime.lineEntry")
	int32Type := ctx.Int32Type()
	int16Type := ctx.Int16Type()

	fileIndices := make(map[string]uint16)
	var fileNames []llvm.Value
	entries := make([]llvm.Value, 0, len(lines))
	for _, line := range lines {
		fileIndex, ok := fileIndices[line.file]
		if !ok {
			if len(fileIndices) == 1<<16 {
				return errors.New("caller line table contains too many files")
			}
			fileIndex = uint16(len(fileIndices))
			fileIndices[line.file] = fileIndex
			fileData := ctx.ConstString(line.file, false)
			fileGlobal := llvm.AddGlobal(mod, fileData.Type(), fmt.Sprintf("runtime.lineFile.%d.%d", generation, fileIndex))
			fileGlobal.SetInitializer(fileData)
			fileGlobal.SetGlobalConstant(true)
			fileGlobal.SetLinkage(llvm.PrivateLinkage)
			fileGlobal.SetUnnamedAddr(true)
			fileGlobal.SetAlignment(1)
			filePointer := llvm.ConstGEP(fileData.Type(), fileGlobal, []llvm.Value{
				llvm.ConstInt(ctx.Int32Type(), 0, false),
				llvm.ConstInt(ctx.Int32Type(), 0, false),
			})
			fileNames = append(fileNames, llvm.ConstNamedStruct(stringType, []llvm.Value{
				llvm.ConstPointerCast(filePointer, ptrType),
				llvm.ConstInt(uintptrType, uint64(len(line.file)), false),
			}))
		}
		for line.length != 0 {
			if line.offset < -1<<31 || line.offset > 1<<31-1 {
				return errors.New("caller line table exceeds the 32-bit text range")
			}
			length := min(line.length, 1<<16-1)
			entries = append(entries, llvm.ConstNamedStruct(lineType, []llvm.Value{
				llvm.ConstInt(int32Type, uint64(uint32(int32(line.offset))), false),
				llvm.ConstInt(int16Type, length, false),
				llvm.ConstInt(int16Type, uint64(fileIndex), false),
				llvm.ConstInt(int32Type, line.line, false),
			}))
			line.offset += int64(length)
			line.length -= length
		}
	}

	filesLengthGlobal.SetInitializer(llvm.ConstInt(uintptrType, uint64(len(fileNames)), false))
	filesArrayType := llvm.ArrayType(stringType, len(fileNames))
	filesArray := llvm.AddGlobal(mod, filesArrayType, fmt.Sprintf("runtime.lineFiles.data.%d", generation))
	filesArray.SetInitializer(llvm.ConstArray(stringType, fileNames))
	filesArray.SetGlobalConstant(true)
	filesArray.SetLinkage(llvm.InternalLinkage)
	filesGlobal.SetInitializer(llvm.ConstGEP(filesArrayType, filesArray, []llvm.Value{
		llvm.ConstInt(ctx.Int32Type(), 0, false),
		llvm.ConstInt(ctx.Int32Type(), 0, false),
	}))

	lengthGlobal.SetInitializer(llvm.ConstInt(uintptrType, uint64(len(entries)), false))
	if len(entries) == 0 {
		tableGlobal.SetInitializer(llvm.ConstPointerNull(ptrType))
		return nil
	}
	arrayType := llvm.ArrayType(lineType, len(entries))
	array := llvm.AddGlobal(mod, arrayType, fmt.Sprintf("runtime.lineTable.data.%d", generation))
	array.SetInitializer(llvm.ConstArray(lineType, entries))
	array.SetGlobalConstant(true)
	array.SetLinkage(llvm.InternalLinkage)
	tableGlobal.SetInitializer(llvm.ConstGEP(arrayType, array, []llvm.Value{
		llvm.ConstInt(ctx.Int32Type(), 0, false),
		llvm.ConstInt(ctx.Int32Type(), 0, false),
	}))
	return nil
}

func callerLinesEqual(left, right []callerLine) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
