package health

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yourname/camctl/internal/camera"
)

// nopConn EMBED interface net.Conn nhưng chỉ định nghĩa Close.
// Các method khác là nil và sẽ panic nếu gọi — chấp nhận được vì Check chỉ gọi Close.
type nopConn struct{ net.Conn }

func (nopConn) Close() error { return nil }

// fakeDialer là Dialer giả: host chứa "down" → lỗi; host chứa "slow" → chờ tới khi ctx hết hạn.
type fakeDialer struct {
	calls atomic.Int32 // atomic vì nhiều goroutine cùng tăng
}

func (d *fakeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d.calls.Add(1)
	switch {
	case strings.Contains(addr, "down"):
		return nil, errors.New("connection refused")
	case strings.Contains(addr, "slow"):
		<-ctx.Done() // block cho tới khi timeout/cancel
		return nil, ctx.Err()
	default:
		return nopConn{}, nil
	}
}

func cams(hosts ...string) []camera.Camera {
	out := make([]camera.Camera, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, camera.Camera{ID: h, Name: h, RTSPURL: "rtsp://" + h + "/live"})
	}
	return out
}

func TestChecker_CheckAll(t *testing.T) {
	d := &fakeDialer{}
	c := New(WithDialer(d), WithWorkers(3), WithTimeout(50*time.Millisecond))

	results := c.CheckAll(context.Background(), cams("ok-2", "down-1", "ok-1", "slow-1"))

	if len(results) != 4 {
		t.Fatalf("want 4 results, got %d", len(results))
	}
	// Sắp theo ID: down-1, ok-1, ok-2, slow-1
	want := map[string]camera.Status{
		"down-1": camera.StatusOffline,
		"ok-1":   camera.StatusOnline,
		"ok-2":   camera.StatusOnline,
		"slow-1": camera.StatusOffline,
	}
	for i, r := range results {
		if want[r.CameraID] != r.Status {
			t.Errorf("%s: want %s got %s (err=%v)", r.CameraID, want[r.CameraID], r.Status, r.Err)
		}
		if i > 0 && results[i-1].CameraID > r.CameraID {
			t.Errorf("results not sorted: %s before %s", results[i-1].CameraID, r.CameraID)
		}
	}
	if got := d.calls.Load(); got != 4 {
		t.Errorf("dialer should be called 4 times, got %d", got)
	}

	// Lỗi timeout phải giữ được context.DeadlineExceeded qua các lớp bọc.
	slow := results[3]
	if !errors.Is(slow.Err, context.DeadlineExceeded) {
		t.Errorf("slow camera should time out, got %v", slow.Err)
	}

	s := Summarize(results)
	if s.Online != 2 || s.Offline != 2 || s.Total != 4 {
		t.Errorf("bad summary: %+v", s)
	}
}

func TestChecker_InvalidURL(t *testing.T) {
	c := New(WithDialer(&fakeDialer{}))
	r := c.Check(context.Background(), camera.Camera{ID: "bad", RTSPURL: "http://x"})
	if r.Status != camera.StatusOffline || !errors.Is(r.Err, camera.ErrInvalidURL) {
		t.Fatalf("got %+v", r)
	}
}

func TestChecker_CancelStopsEarly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel trước khi chạy

	c := New(WithDialer(&fakeDialer{}), WithWorkers(2))
	results := c.CheckAll(ctx, cams("ok-1", "ok-2", "ok-3"))
	if len(results) == 3 {
		t.Fatalf("producer should stop early when ctx is cancelled")
	}
}

func TestChecker_Watch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := New(WithDialer(&fakeDialer{}))
	ch := c.Watch(ctx, cams("ok-1"), 5*time.Millisecond)

	for i := 0; i < 3; i++ {
		select {
		case results := <-ch:
			if len(results) != 1 {
				t.Fatalf("round %d: got %d results", i, len(results))
			}
		case <-time.After(time.Second):
			t.Fatal("watch produced nothing")
		}
	}

	cancel()
	// Channel phải được close sau khi cancel → nhận được zero value với ok=false.
	select {
	case _, ok := <-ch:
		for ok { // drain phòng khi còn 1 lượt đang gửi
			_, ok = <-ch
		}
	case <-time.After(time.Second):
		t.Fatal("watch channel was not closed after cancel")
	}
}
