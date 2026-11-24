package os_test

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestReadAtOverlapped(t *testing.T) {
	name := filepath.Join(t.TempDir(), "readat")
	if err := os.WriteFile(name, []byte("abcdef"), 0666); err != nil {
		t.Fatal(err)
	}
	path, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(path, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		t.Fatal(err)
	}
	f := os.NewFile(uintptr(h), name)
	defer f.Close()

	b := make([]byte, 4)
	if n, err := f.ReadAt(b, 4); n != 2 || err != io.EOF || string(b[:n]) != "ef" {
		t.Fatalf("ReadAt(4) = %q, %d, %v; want ef, 2, EOF", b[:n], n, err)
	}
	if n, err := f.ReadAt(b, 1<<32); n != 0 || err != io.EOF {
		t.Fatalf("ReadAt(1<<32) = %d, %v; want 0, EOF", n, err)
	}
}
