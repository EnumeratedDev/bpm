package bpmlib

import (
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"syscall"
	"time"

	version "github.com/knqyf263/go-rpm-version"
	"github.com/schollz/progressbar/v3"
)

type BPMLock struct {
	file *os.File
	path string
}

func LockBPM(rootDir string) (*BPMLock, error) {
	// Create parent directories if they don't already exist
	err := os.MkdirAll(path.Join(rootDir, "/var/lib/bpm"), 0755)
	if err != nil {
		return nil, err
	}

	// Create file
	f, err := os.Create(path.Join(rootDir, "var/lib/bpm/bpm.lock"))
	if err != nil {
		return nil, err
	}

	// Get exclusive file lock on file
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		return nil, err
	}

	return &BPMLock{f, path.Join(rootDir, "var/lib/bpm/bpm.lock")}, nil
}

func (lock *BPMLock) Unlock() error {
	err := lock.file.Close()
	if err != nil {
		return err
	}

	err = os.Remove(lock.path)
	if err != nil {
		return err
	}

	return nil
}

func GetArch() string {
	uname := syscall.Utsname{}
	err := syscall.Uname(&uname)
	if err != nil {
		return "unknown"
	}

	var byteString [65]byte
	var indexLength int
	for ; uname.Machine[indexLength] != 0; indexLength++ {
		byteString[indexLength] = uint8(uname.Machine[indexLength])
	}
	return string(byteString[:indexLength])
}

func CompareVersions(version1, version2 string) int {
	v1 := version.NewVersion(version1)
	v2 := version.NewVersion(version2)

	return v1.Compare(v2)
}

func createProgressBar(max int64, description string, hideBar bool) *progressbar.ProgressBar {
	var output io.Writer
	if hideBar {
		output = io.Discard
	} else {
		output = os.Stderr
	}

	if len(description) < 40 {
		for i := len(description); i < 40; i++ {
			description += " "
		}
	}
	if len(description) > 40 {
		overflow := len(description) - 37
		description = description[:len(description)-overflow] + "..."
	}

	return progressbar.NewOptions64(max,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWriter(output),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowTotalBytes(true),
		progressbar.OptionSetWidth(20),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(output, "\n")
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionSetTheme(progressbar.ThemeASCII))
}

func stringSliceRemove(s []string, r string) []string {
	for i, v := range s {
		if v == r {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}

func unsignedBytesToHumanReadable(b uint64) string {
	bf := float64(b)
	for _, unit := range []string{"", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi"} {
		if math.Abs(bf) < 1024.0 {
			return fmt.Sprintf("%3.1f%sB", bf, unit)
		}
		bf /= 1024.0
	}
	return fmt.Sprintf("%.1fYiB", bf)
}

func bytesToHumanReadable(b int64) string {
	bf := float64(b)
	for _, unit := range []string{"", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi"} {
		if math.Abs(bf) < 1024.0 {
			return fmt.Sprintf("%3.1f%sB", bf, unit)
		}
		bf /= 1024.0
	}
	return fmt.Sprintf("%.1fYiB", bf)
}

func removeDuplicates[T comparable](sliceList []T) []T {
	allKeys := make(map[T]bool)
	list := []T{}
	for _, item := range sliceList {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}
