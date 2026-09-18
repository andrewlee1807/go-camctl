// Package store lưu trữ danh mục camera.
//
// Cung cấp interface Store và hai implementation: Memory (map trong RAM, an toàn
// cho concurrency) và File (đọc/ghi JSON, tái sử dụng Memory qua embedding).
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/yourname/camctl/internal/camera"
)

// Store là contract mà phần còn lại của chương trình phụ thuộc vào.
// Interface trong Go được thoả mãn NGẦM ĐỊNH: bất kỳ type nào có đủ các method
// này là một Store, không cần khai báo "implements". Đây là duck typing có kiểm
// tra lúc compile — sau này thay bằng Postgres chỉ cần viết type mới.
//
// Quy ước: mọi hàm có I/O nhận context.Context làm tham số đầu tiên.
type Store interface {
	List(ctx context.Context) ([]camera.Camera, error)
	Get(ctx context.Context, id string) (camera.Camera, error)
	Put(ctx context.Context, cam camera.Camera) error
	Delete(ctx context.Context, id string) error
}

// Memory là store trong RAM. map trong Go KHÔNG thread-safe, nên phải bọc mutex.
type Memory struct {
	mu   sync.RWMutex
	cams map[string]camera.Camera
}

// NewMemory là constructor theo quy ước Go: hàm New<Type> trả về *Type.
// map phải được make trước khi ghi; ghi vào nil map sẽ panic.
func NewMemory() *Memory {
	return &Memory{cams: make(map[string]camera.Camera)}
}

// List trả về các camera sắp theo ID. Thứ tự duyệt map trong Go là NGẪU NHIÊN,
// nên phải sort nếu muốn output ổn định.
func (m *Memory) List(ctx context.Context) ([]camera.Camera, error) {
	m.mu.RLock()
	defer m.mu.RUnlock() // defer chạy khi hàm return, tương đương `with lock:`

	out := make([]camera.Camera, 0, len(m.cams))
	for _, c := range m.cams {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b camera.Camera) int {
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

// Get tìm camera theo id; trả camera.ErrNotFound (đã bọc thêm ngữ cảnh) nếu không có.
func (m *Memory) Get(ctx context.Context, id string) (camera.Camera, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c, ok := m.cams[id] // "comma ok" idiom: ok=false nếu key không tồn tại
	if !ok {
		return camera.Camera{}, fmt.Errorf("get %q: %w", id, camera.ErrNotFound)
	}
	return c, nil
}

// Put thêm hoặc ghi đè camera (upsert) sau khi validate.
func (m *Memory) Put(ctx context.Context, cam camera.Camera) error {
	cam.Normalize() // cam là bản copy (value param) nên sửa ở đây không ảnh hưởng caller
	if err := cam.Validate(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.cams[cam.ID] = cam
	return nil
}

// Delete xoá camera theo id.
func (m *Memory) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.cams[id]; !ok {
		return fmt.Errorf("delete %q: %w", id, camera.ErrNotFound)
	}
	delete(m.cams, id)
	return nil
}

// File là store đọc/ghi file JSON.
//
// Nó EMBED *Memory: mọi method của Memory tự động được "promote" lên File
// (File.List, File.Get...). Đây là composition thay cho inheritance của Python —
// không có super(), không có class hierarchy; muốn "override" thì định nghĩa lại
// method cùng tên trên File, như Put và Delete bên dưới.
type File struct {
	*Memory
	path string
}

// OpenFile mở registry từ file JSON. File chưa tồn tại được coi là registry rỗng.
func OpenFile(path string) (*File, error) {
	f := &File{Memory: NewMemory(), path: path}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cams []camera.Camera
	if err := json.Unmarshal(data, &cams); err != nil { // truyền POINTER để Unmarshal ghi vào
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	for _, c := range cams {
		if err := f.Memory.Put(context.Background(), c); err != nil {
			return nil, fmt.Errorf("load %s: %w", path, err)
		}
	}
	return f, nil
}

// Put ghi vào RAM rồi lưu xuống đĩa.
func (f *File) Put(ctx context.Context, cam camera.Camera) error {
	if err := f.Memory.Put(ctx, cam); err != nil {
		return err
	}
	return f.flush(ctx)
}

// Delete xoá khỏi RAM rồi lưu xuống đĩa.
func (f *File) Delete(ctx context.Context, id string) error {
	if err := f.Memory.Delete(ctx, id); err != nil {
		return err
	}
	return f.flush(ctx)
}

// flush ghi toàn bộ registry ra file theo kiểu atomic: ghi file tạm rồi rename,
// để không bao giờ để lại file JSON viết dở nếu chương trình chết giữa chừng.
// Tên viết thường = unexported (private trong package).
func (f *File) flush(ctx context.Context) error {
	cams, err := f.Memory.List(ctx)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cams, "", "  ")
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, f.path); err != nil {
		return fmt.Errorf("rename %s: %w", tmp, err)
	}
	return nil
}

// Kiểm tra lúc compile rằng cả hai type đều thoả mãn Store.
// Nếu thiếu method, dòng này sẽ báo lỗi biên dịch ngay — idiom rất phổ biến.
var (
	_ Store = (*Memory)(nil)
	_ Store = (*File)(nil)
)
