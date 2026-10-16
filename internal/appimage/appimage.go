// Package appimage reads what AiM needs from an AppImage file without
// running it (ADR 0007).
package appimage

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path"
	"strings"

	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/squashfs"
)

// ErrNotAppImage means the file is not an AppImage AiM can read: either not
// an AppImage at all, or a legacy type-1 AppImage.
var ErrNotAppImage = errors.New("not a type-2 AppImage")

// Info is what an AppImage carries about itself.
type Info struct {
	// DesktopFile is the Embedded Desktop Entry's filename, e.g.
	// "md.obsidian.Obsidian.desktop"; empty when there is none.
	DesktopFile  string
	DesktopEntry []byte
	// Icon is the AppImage's own icon (.DirIcon); IconExt is ".png" or
	// ".svg". Both are empty when there is no usable icon.
	Icon    []byte
	IconExt string
	// UpdateInfo is the runtime's embedded update information, e.g.
	// "gh-releases-zsync|owner|repo|latest|App-*.AppImage.zsync"; empty
	// when there is none.
	UpdateInfo string
}

// Inspect reads an AppImage's embedded metadata.
func Inspect(path string) (Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, err
	}
	defer f.Close()
	offset, err := squashfsOffset(f)
	if err != nil {
		return Info{}, fmt.Errorf("%s: %w", path, err)
	}
	stat, err := f.Stat()
	if err != nil {
		return Info{}, err
	}
	fs, err := squashfs.Read(file.New(f, true), stat.Size()-offset, offset, 0)
	if err != nil {
		return Info{}, fmt.Errorf("%s: reading squashfs: %w", path, err)
	}

	info := Info{UpdateInfo: updateInfo(f)}
	entries, err := fs.ReadDir(".")
	if err != nil {
		return Info{}, err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".desktop") {
			info.DesktopFile = e.Name()
			if info.DesktopEntry, err = fs.ReadFile(e.Name()); err != nil {
				return Info{}, err
			}
			break
		}
	}
	if icon, err := readFollowingLinks(fs, ".DirIcon"); err == nil {
		if ext := iconExt(icon); ext != "" {
			info.Icon, info.IconExt = icon, ext
		}
	}
	return info, nil
}

// updateInfo reads the runtime's .upd_info ELF section. The section is a
// fixed-size buffer, padded with NUL bytes.
func updateInfo(f *os.File) string {
	ef, err := elf.NewFile(f)
	if err != nil {
		return ""
	}
	section := ef.Section(".upd_info")
	if section == nil {
		return ""
	}
	data, err := section.Data()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(bytes.TrimRight(data, "\x00")))
}

// maxLinks bounds symlink chains, so a link loop can't hang Inspect.
const maxLinks = 10

// readFollowingLinks reads a file inside the AppImage, following symlinks.
func readFollowingLinks(fs *squashfs.FileSystem, name string) ([]byte, error) {
	for range maxLinks {
		target, isLink, err := readlink(fs, name)
		if err != nil {
			return nil, err
		}
		if !isLink {
			return fs.ReadFile(name)
		}
		if strings.HasPrefix(target, "/") {
			name = strings.TrimPrefix(target, "/")
		} else {
			name = path.Join(path.Dir(name), target)
		}
	}
	return nil, fmt.Errorf("%s: too many symlinks", name)
}

// readlink returns the target of name when it is a symlink.
func readlink(fs *squashfs.FileSystem, name string) (target string, isLink bool, err error) {
	entries, err := fs.ReadDir(path.Dir(name))
	if err != nil {
		return "", false, err
	}
	for _, e := range entries {
		if e.Name() != path.Base(name) {
			continue
		}
		link, ok := e.(interface{ Readlink() (string, error) })
		if e.Type()&iofs.ModeSymlink == 0 || !ok {
			return "", false, nil
		}
		target, err := link.Readlink()
		return target, true, err
	}
	return "", false, fmt.Errorf("%s: %w", name, iofs.ErrNotExist)
}

// iconExt recognises the image formats Desktop Entries can use.
func iconExt(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return ".png"
	case bytes.Contains(data[:min(len(data), 1024)], []byte("<svg")):
		return ".svg"
	}
	return ""
}

// squashfsOffset checks the type-2 AppImage magic and returns where the
// squashfs starts: right after the runtime's ELF section header table.
func squashfsOffset(f *os.File) (int64, error) {
	header := make([]byte, 64)
	if _, err := f.ReadAt(header, 0); err != nil {
		return 0, ErrNotAppImage
	}
	if string(header[:4]) != "\x7fELF" || header[4] != 2 || string(header[8:11]) != "AI\x02" {
		return 0, ErrNotAppImage
	}
	le := binary.LittleEndian
	shoff := le.Uint64(header[40:48])
	shentsize, shnum := le.Uint16(header[58:60]), le.Uint16(header[60:62])
	return int64(shoff) + int64(shentsize)*int64(shnum), nil
}
