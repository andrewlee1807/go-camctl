package store_test // external test package: chỉ dùng được API public, giống người dùng thật

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/yourname/camctl/internal/camera"
	"github.com/yourname/camctl/internal/store"
)

func sample(id string) camera.Camera {
	return camera.Camera{ID: id, Name: "Cam " + id, RTSPURL: "rtsp://10.0.0.1/" + id, Zone: "north"}
}

// testStore là "contract test": cùng một bộ kiểm tra chạy cho MỌI implementation của Store.
// Nhờ interface, sau này thêm PostgresStore chỉ cần gọi thêm testStore(t, newPostgres).
func testStore(t *testing.T, newStore func(t *testing.T) store.Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("put then get", func(t *testing.T) {
		st := newStore(t)
		if err := st.Put(ctx, sample("a")); err != nil {
			t.Fatal(err)
		}
		got, err := st.Get(ctx, "a")
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "Cam a" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("get missing returns ErrNotFound", func(t *testing.T) {
		st := newStore(t)
		_, err := st.Get(ctx, "nope")
		if !errors.Is(err, camera.ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})

	t.Run("put invalid is rejected", func(t *testing.T) {
		st := newStore(t)
		err := st.Put(ctx, camera.Camera{ID: "x"})
		var verr *camera.ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("want ValidationError, got %v", err)
		}
	})

	t.Run("list is sorted by id", func(t *testing.T) {
		st := newStore(t)
		for _, id := range []string{"c", "a", "b"} {
			if err := st.Put(ctx, sample(id)); err != nil {
				t.Fatal(err)
			}
		}
		cams, err := st.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(cams) != 3 || cams[0].ID != "a" || cams[2].ID != "c" {
			t.Fatalf("unexpected order: %+v", cams)
		}
	})

	t.Run("delete", func(t *testing.T) {
		st := newStore(t)
		_ = st.Put(ctx, sample("a"))
		if err := st.Delete(ctx, "a"); err != nil {
			t.Fatal(err)
		}
		if err := st.Delete(ctx, "a"); !errors.Is(err, camera.ErrNotFound) {
			t.Fatalf("second delete should be ErrNotFound, got %v", err)
		}
	})
}

func TestMemory(t *testing.T) {
	testStore(t, func(t *testing.T) store.Store { return store.NewMemory() })
}

func TestFile(t *testing.T) {
	testStore(t, func(t *testing.T) store.Store {
		// t.TempDir() tự dọn dẹp khi test kết thúc.
		f, err := store.OpenFile(filepath.Join(t.TempDir(), "cams.json"))
		if err != nil {
			t.Fatal(err)
		}
		return f
	})
}

// TestFile_Persist kiểm tra riêng phần File thêm vào: dữ liệu sống qua lần mở lại.
func TestFile_Persist(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cams.json")

	f1, err := store.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f1.Put(ctx, sample("a")); err != nil {
		t.Fatal(err)
	}

	f2, err := store.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f2.Get(ctx, "a"); err != nil {
		t.Fatalf("camera should survive reopen: %v", err)
	}
}

// TestMemory_Concurrent chạy với `go test -race` để bắt data race trên map.
func TestMemory_Concurrent(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			id := string(rune('a' + i))
			_ = st.Put(ctx, sample(id))
			_, _ = st.List(ctx)
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}
