package version

import (
	"fmt"
	"strconv"
	"strings"
)

const sstSuffix = "sst"
const TmpSuffix = "tmp"

const Lock = "LOCK"
const Options = "OPTIONS"
const manifestPrefix = "MANIFEST-"

// FileType represents a file type.
type FileType int

// File types.
const (
	TypeManifest FileType = iota
	TypeJournal
	TypeTable
	TypeTemp
	TypeInfo
)

// FileDesc represents file type and file number
type FileDesc struct {
	FileType   FileType
	FileNumber int64
}

// current returns current file name for saving manifest file name
func current() string {
	return "CURRENT"
}

// Table returns the sst's file name
func Table(fileNumber int64) string {
	return fmt.Sprintf("%06d.%s", fileNumber, sstSuffix)
}

// manifestFileName returns manifest file name
func manifestFileName(fileNumber int64) string {
	return fmt.Sprintf("%s%06d", manifestPrefix, fileNumber)
}

// ParseFileName parses file name.
// if the file name was successfully parsed, returns file desc instance, else return nil.
func ParseFileName(fileName string) *FileDesc {
	if strings.HasSuffix(fileName, ".sst") {
		n, err := strconv.ParseInt(removeSuffix(fileName, ".sst"), 10, 64)
		if err != nil {
			return nil
		}
		return &FileDesc{
			FileType:   TypeTable,
			FileNumber: n,
		}
	}
	return nil
}

// removeSuffix removes suffix, then returns new string
func removeSuffix(value, suffix string) string {
	return value[0 : len(value)-len(suffix)]
}
