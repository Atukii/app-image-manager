package appimage_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/squashfs"
)

// fixture describes a fake type-2 AppImage for tests.
type fixture struct {
	files      map[string]string // path in the AppImage → content
	symlinks   map[string]string // path in the AppImage → link target
	updateInfo string            // contents of the ELF .upd_info section
}

// build writes the fixture as an AppImage file: a minimal ELF runtime
// carrying the AppImage type-2 magic and a .upd_info section, followed by a
// gzip squashfs filesystem, the same layout real AppImages have.
func (f fixture) build(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.AppImage")
	runtime := elfRuntime(f.updateInfo)
	if err := os.WriteFile(path, runtime, 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	const maxSize = 16 << 20
	fs, err := squashfs.Create(file.New(out, false), maxSize, int64(len(runtime)), 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(fs.Workspace()) })
	for name, content := range f.files {
		if dir := filepath.Dir(name); dir != "." {
			if err := fs.Mkdir(dir); err != nil {
				t.Fatalf("mkdir %s: %v", dir, err)
			}
		}
		w, err := fs.OpenFile(name, os.O_CREATE|os.O_RDWR)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	// go-diskfs can't create symlinks itself, but Finalize copies the ones
	// it finds in its workspace.
	for name, target := range f.symlinks {
		if err := os.Symlink(target, filepath.Join(fs.Workspace(), name)); err != nil {
			t.Fatalf("symlink %s: %v", name, err)
		}
	}
	// Finalize reads symlinks relative to the current directory instead of
	// its workspace (go-diskfs v1.9.4), so run it from the workspace.
	t.Chdir(fs.Workspace())
	if err := fs.Finalize(squashfs.FinalizeOptions{}); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	return path
}

// elfRuntime builds a minimal 64-bit ELF file whose only sections are the
// string table and .upd_info. The section header table comes last, so the
// squashfs starts right after it, as in a real AppImage runtime.
func elfRuntime(updateInfo string) []byte {
	const ehsize, shentsize = 64, 64
	shstrtab := []byte("\x00.shstrtab\x00.upd_info\x00")
	upd := append([]byte(updateInfo), 0)

	var b bytes.Buffer
	shoff := uint64(ehsize + len(shstrtab) + len(upd))
	ident := [16]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0, 'A', 'I', 2}
	b.Write(ident[:])
	le := binary.LittleEndian
	for _, v := range []any{
		uint16(2), uint16(62), uint32(1), // ET_EXEC, x86-64, version
		uint64(0), uint64(0), shoff, // entry, phoff, shoff
		uint32(0), uint16(ehsize), uint16(0), uint16(0), // flags, ehsize, phentsize, phnum
		uint16(shentsize), uint16(3), uint16(1), // shentsize, shnum, shstrndx
	} {
		binary.Write(&b, le, v)
	}
	b.Write(shstrtab)
	b.Write(upd)
	section := func(name, typ uint32, off, size uint64) {
		for _, v := range []any{name, typ, uint64(0), uint64(0), off, size, uint32(0), uint32(0), uint64(1), uint64(0)} {
			binary.Write(&b, le, v)
		}
	}
	section(0, 0, 0, 0)                                                   // null section
	section(1, 3, ehsize, uint64(len(shstrtab)))                          // .shstrtab
	section(11, 1, uint64(ehsize+len(shstrtab)), uint64(len(updateInfo))) // .upd_info
	return b.Bytes()
}
