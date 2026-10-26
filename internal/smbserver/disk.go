package smbserver

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	filesystem "github.com/go-filesystems/interface"
)

// Both layers are read-only: the SMB share and this filesystem adapter.
// os.Root prevents symlinks from escaping the selected directory.
type disk struct {
	root   *os.Root
	log    *eventLog
	mu     sync.Mutex
	closed bool
	files  sync.WaitGroup
}

func newDisk(path string, l *eventLog) (*disk, error) {
	r, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	return &disk{root: r, log: l}, nil
}

func localName(p string) (string, error) {
	if strings.ContainsAny(p, "\\:\x00") {
		return "", fs.ErrPermission
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return "", fs.ErrPermission
		}
	}
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		p = "."
	}
	p = filepath.FromSlash(p)
	if !filepath.IsLocal(p) {
		return "", fs.ErrPermission
	}
	return p, nil
}

// pathKey makes logged path IDs unguessable from common names and unlinkable
// across launches, while one launch still correlates reads of the same file.
var pathKey = func() []byte {
	k := make([]byte, 32)
	rand.Read(k)
	return k
}()

func pathID(p string) string {
	m := hmac.New(sha256.New, pathKey)
	m.Write([]byte(p))
	return fmt.Sprintf("%x", m.Sum(nil)[:6])
}
func (d *disk) Close() error {
	d.mu.Lock()
	d.closed = true
	err := d.root.Close()
	d.mu.Unlock()
	d.files.Wait()
	return err
}
func (d *disk) Stat(p string) (filesystem.Stat, error) {
	n, err := localName(p)
	if err != nil {
		return nil, err
	}
	i, err := d.root.Stat(n)
	if err != nil {
		return nil, err
	}
	var mode uint16 = 0100444
	if i.IsDir() {
		mode = 0040555
	} else if !i.Mode().IsRegular() {
		return nil, fs.ErrPermission
	}
	return filesystem.NewStat(mode, uint64(i.Size()), inode(n)), nil
}
func inode(p string) uint64 {
	h := sha256.Sum256([]byte(p))
	var n uint64
	for _, b := range h[:8] {
		n = n<<8 | uint64(b)
	}
	return n
}
func (d *disk) ListDir(p string) ([]filesystem.DirEntry, error) {
	n, err := localName(p)
	if err != nil {
		return nil, err
	}
	f, err := d.root.Open(n)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	result := make([]filesystem.DirEntry, 0, len(entries))
	for _, e := range entries {
		child := filepath.Join(n, e.Name())
		// The SMB server stats every entry it lists, so only links need
		// resolving here. Root refuses the ones that point outside the share.
		mode := e.Type()
		if mode&fs.ModeSymlink != 0 {
			i, err := d.root.Stat(child)
			if err != nil {
				continue
			}
			mode = i.Mode().Type()
		}
		var typ uint8 = 1 // Linux dirent type used by the filesystem interface.
		if mode.IsDir() {
			typ = 2
		} else if !mode.IsRegular() {
			continue
		}
		result = append(result, filesystem.NewDirEntry(inode(child), e.Name(), typ))
	}
	return result, nil
}
func (d *disk) OpenFile(p string) (filesystem.File, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, fs.ErrClosed
	}
	n, err := localName(p)
	if err != nil {
		return nil, err
	}
	// Check type before opening to avoid blocking on ordinary named pipes.
	i, err := d.root.Stat(n)
	if err != nil {
		return nil, err
	}
	if !i.Mode().IsRegular() {
		return nil, fs.ErrPermission
	}
	f, err := d.root.Open(n)
	if err != nil {
		return nil, err
	}
	i, err = f.Stat()
	if err != nil || !i.Mode().IsRegular() {
		f.Close()
		if err != nil {
			return nil, err
		}
		return nil, fs.ErrPermission
	}
	id := pathID(p)
	d.log.event("file_open", map[string]any{"file_id": id, "size": i.Size()})
	d.files.Add(1)
	return &diskFile{f: f, size: i.Size(), id: id, log: d.log, release: d.files.Done}, nil
}
func (d *disk) ReadFile(p string) ([]byte, error) {
	f, err := d.OpenFile(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// Never let an upstream fallback load a movie into RAM.
	if f.Size() > 8<<20 {
		return nil, fs.ErrPermission
	}
	return io.ReadAll(io.NewSectionReader(f, 0, f.Size()))
}
func (d *disk) ReadLink(string) (string, error)             { return "", fs.ErrPermission }
func (d *disk) WriteFile(string, []byte, os.FileMode) error { return fs.ErrPermission }
func (d *disk) MkDir(string, os.FileMode) error             { return fs.ErrPermission }
func (d *disk) DeleteFile(string) error                     { return fs.ErrPermission }
func (d *disk) DeleteDir(string) error                      { return fs.ErrPermission }
func (d *disk) Rename(string, string) error                 { return fs.ErrPermission }

type diskFile struct {
	f        *os.File
	size     int64
	id       string
	log      *eventLog
	reads    atomic.Uint64
	bytes    atomic.Uint64
	once     sync.Once
	closeErr error
	release  func()
}

func (f *diskFile) Size() int64 { return f.size }
func (f *diskFile) ReadAt(p []byte, off int64) (int, error) {
	n, err := f.f.ReadAt(p, off)
	count := f.reads.Add(1)
	f.bytes.Add(uint64(n))
	if count <= 4 || count%256 == 0 || (err != nil && err != io.EOF) {
		e := ""
		if err != nil {
			e = "io_error"
			if err == io.EOF {
				e = "EOF"
			}
		}
		f.log.event("file_read", map[string]any{"file_id": f.id, "offset": off, "requested": len(p), "bytes": n, "read_number": count, "result": e})
	}
	return n, err
}
func (f *diskFile) Close() error {
	f.once.Do(func() {
		f.closeErr = f.f.Close()
		f.log.event("file_closed", map[string]any{"file_id": f.id, "read_calls": f.reads.Load(), "bytes": f.bytes.Load()})
		if f.release != nil {
			f.release()
		}
	})
	return f.closeErr
}

var _ filesystem.Filesystem = (*disk)(nil)
var _ filesystem.Opener = (*disk)(nil)
