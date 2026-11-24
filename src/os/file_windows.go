//go:build windows

// Portions copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"internal/syscall/windows"
	"io"
	"runtime"
	"sync"
	"syscall"
	"unicode/utf16"
)

const DevNull = "NUL"

type syscallFd = syscall.Handle

// Symlink is a stub, it is not implemented.
func Symlink(oldname, newname string) error {
	return ErrNotImplemented
}

// Readlink is a stub (for now), always returning the string it was given
func Readlink(name string) (string, error) {
	return name, nil
}

func rename(oldname, newname string) error {
	e := windows.Rename(fixLongPath(oldname), fixLongPath(newname))
	if e != nil {
		return &LinkError{"rename", oldname, newname, e}
	}
	return nil
}

type file struct {
	handle     FileHandle
	name       string
	appendMode bool
	dirinfo    *dirInfo
}

func (f *file) close() error {
	f.dirinfo = nil
	return f.handle.Close()
}

func NewFile(fd uintptr, name string) *File {
	h := unixFileHandle(fd)
	var handle FileHandle = h
	if kind, err := syscall.GetFileType(syscall.Handle(fd)); err == nil && kind == syscall.FILE_TYPE_DISK {
		handle = &windowsDiskFile{unixFileHandle: h}
	}
	return &File{&file{handle: handle, name: name}}
}

func Pipe() (r *File, w *File, err error) {
	var p [2]syscall.Handle
	e := handleSyscallError(syscall.Pipe(p[:]))
	if e != nil {
		return nil, nil, err
	}
	r = NewFile(uintptr(p[0]), "|0")
	w = NewFile(uintptr(p[1]), "|1")
	return
}

func (f *unixFileHandle) Truncate(size int64) error {
	return ErrNotImplemented
}

// Truncate changes the size of the named file.
// If the file is a symbolic link, it changes the size of the link's target.
func Truncate(name string, size int64) error {
	return &PathError{Op: "truncate", Path: name, Err: ErrNotImplemented}
}

func tempDir() string {
	n := uint32(syscall.MAX_PATH)
	for {
		b := make([]uint16, n)
		n, _ = syscall.GetTempPath(uint32(len(b)), &b[0])
		if n > uint32(len(b)) {
			continue
		}
		if n == 3 && b[1] == ':' && b[2] == '\\' {
			// Do nothing for path, like C:\.
		} else if n > 0 && b[n-1] == '\\' {
			// Otherwise remove terminating \.
			n--
		}
		return string(utf16.Decode(b[:n]))
	}
}

func (f unixFileHandle) ReadAt(b []byte, offset int64) (int, error) {
	if _, err := syscall.GetFileType(syscallFd(f)); err != nil {
		return 0, handleSyscallError(err)
	}
	return 0, syscall.ESPIPE
}

type windowsDiskFile struct {
	unixFileHandle
	mu sync.Mutex
}

func (f *windowsDiskFile) Read(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.unixFileHandle.Read(b)
}

func (f *windowsDiskFile) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.unixFileHandle.Write(b)
}

func (f *windowsDiskFile) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.unixFileHandle.Seek(offset, whence)
}

func (f *windowsDiskFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.unixFileHandle.Close()
}

func (f *windowsDiskFile) ReadAt(b []byte, offset int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	fd := syscallFd(f.unixFileHandle)
	nonblocking, err := windows.IsNonblock(fd)
	if err != nil {
		return 0, err
	}
	const maxRead = 1 << 30
	if len(b) > maxRead {
		b = b[:maxRead]
	}

	if !nonblocking {
		// Synchronous handles update the file pointer during ReadFile.
		// https://learn.microsoft.com/windows/win32/api/fileapi/nf-fileapi-readfile
		current, err := f.unixFileHandle.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, err
		}
		if _, err := f.unixFileHandle.Seek(offset, io.SeekStart); err != nil {
			return 0, err
		}
		n, err := f.unixFileHandle.Read(b)
		_, seekErr := f.unixFileHandle.Seek(current, io.SeekStart)
		if seekErr != nil && (err == nil || err == io.EOF) {
			err = seekErr
		}
		return n, err
	}

	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return 0, err
	}
	defer syscall.CloseHandle(event)
	overlapped := syscall.Overlapped{
		Offset:     uint32(offset),
		OffsetHigh: uint32(offset >> 32),
		HEvent:     event | 1, // Suppress notification to any external completion port.
	}
	var n uint32
	err = syscall.ReadFile(fd, b, &n, &overlapped)
	if err == syscall.ERROR_IO_PENDING {
		err = windows.GetOverlappedResult(fd, &overlapped, &n, true)
	}
	runtime.KeepAlive(b)
	if err == syscall.ERROR_HANDLE_EOF || err == nil && n == 0 && len(b) != 0 {
		return int(n), io.EOF
	}
	return int(n), handleSyscallError(err)
}

// WriteAt writes len(b) bytes to the File starting at byte offset off.
// It returns the number of bytes written and an error, if any.
// WriteAt returns a non-nil error when n != len(b).
//
// If file was opened with the O_APPEND flag, WriteAt returns an error.
//
// TODO: move to file_anyos once WriteAt is implemented for windows.
func (f unixFileHandle) WriteAt(b []byte, offset int64) (n int, err error) {
	return -1, ErrNotImplemented
}

// Seek wraps syscall.Seek.
func (f unixFileHandle) Seek(offset int64, whence int) (int64, error) {
	newoffset, err := syscall.Seek(syscallFd(f), offset, whence)
	return newoffset, handleSyscallError(err)
}

func (f unixFileHandle) Sync() error {
	return ErrNotImplemented
}

// Truncate changes the size of the named file.
// If the file is a symbolic link, it changes the size of the link's target.
func (f *File) Truncate(size int64) error {
	if f.handle == nil {
		return &PathError{Op: "truncate", Path: f.name, Err: ErrClosed}
	}
	return Truncate(f.name, size)
}

// isWindowsNulName reports whether name is os.DevNull ('NUL') on Windows.
// True is returned if name is 'NUL' whatever the case.
func isWindowsNulName(name string) bool {
	if len(name) != 3 {
		return false
	}
	if name[0] != 'n' && name[0] != 'N' {
		return false
	}
	if name[1] != 'u' && name[1] != 'U' {
		return false
	}
	if name[2] != 'l' && name[2] != 'L' {
		return false
	}
	return true
}

func (f *File) chmod(mode FileMode) error {
	return ErrNotImplemented
}

func (f *File) chdir() error {
	if err := syscall.Fchdir(syscall.Handle(f.Fd())); err != nil {
		return &PathError{Op: "chdir", Path: f.name, Err: handleSyscallError(err)}
	}
	return nil
}
