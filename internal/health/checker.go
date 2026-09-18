// Package health kiểm tra camera có đang lắng nghe cổng RTSP hay không.
//
// Đây là phần minh hoạ concurrency: goroutine, channel, select, WaitGroup,
// context timeout/cancel, và pattern worker pool.
package health

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/yourname/camctl/internal/camera"
)

// Dialer là phần nhỏ nhất của *net.Dialer mà Checker cần.
// Khai báo interface ở phía NGƯỜI DÙNG (consumer) thay vì phía cung cấp là
// idiom Go: *net.Dialer thoả mãn nó sẵn, còn trong test ta cắm một fake vào.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Result là kết quả kiểm tra một camera.
// `json:"-"` bỏ qua field Err khi encode (error là interface, encode ra {} vô nghĩa).
type Result struct {
	CameraID string        `json:"camera_id"`
	Status   camera.Status `json:"status"`
	Latency  time.Duration `json:"latency_ns"`
	Err      error         `json:"-"`
}

// Checker cấu hình cách kiểm tra. Các field unexported: chỉ đổi được qua Option.
type Checker struct {
	dialer  Dialer
	timeout time.Duration
	workers int
}

// Option là "functional option": một closure sửa *Checker.
// Đây là cách Go thay cho keyword arguments có default của Python.
type Option func(*Checker)

// WithTimeout đặt timeout dial cho MỖI camera.
func WithTimeout(d time.Duration) Option {
	return func(c *Checker) { c.timeout = d }
}

// WithWorkers đặt số goroutine kiểm tra song song.
func WithWorkers(n int) Option {
	return func(c *Checker) {
		if n > 0 {
			c.workers = n
		}
	}
}

// WithDialer thay Dialer (dùng trong test).
func WithDialer(d Dialer) Option {
	return func(c *Checker) { c.dialer = d }
}

// New tạo Checker với default hợp lý rồi áp các option theo thứ tự.
func New(opts ...Option) *Checker {
	c := &Checker{
		dialer:  &net.Dialer{},
		timeout: 3 * time.Second,
		workers: 8,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Check kiểm tra một camera: mở TCP tới host:port RTSP rồi đóng ngay.
func (c *Checker) Check(ctx context.Context, cam camera.Camera) Result {
	res := Result{CameraID: cam.ID}

	addr, err := cam.Addr()
	if err != nil {
		res.Status, res.Err = camera.StatusOffline, err
		return res
	}

	// Context con có deadline; cancel() giải phóng tài nguyên dù thành công hay thất bại.
	// Luôn `defer cancel()` ngay sau WithTimeout/WithCancel — go vet sẽ nhắc nếu quên.
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	start := time.Now()
	conn, err := c.dialer.DialContext(ctx, "tcp", addr)
	res.Latency = time.Since(start)
	if err != nil {
		res.Status = camera.StatusOffline
		res.Err = fmt.Errorf("check %s: %w", cam.ID, err)
		return res
	}
	_ = conn.Close() // bỏ qua lỗi Close một cách TƯỜNG MINH (gán cho _)

	res.Status = camera.StatusOnline
	return res
}

// CheckAll kiểm tra nhiều camera song song bằng worker pool.
//
// Sơ đồ:  producer ──jobs──▶ N workers ──results──▶ collector (goroutine hiện tại)
//
// Kết quả trả về được sort theo CameraID để ổn định (goroutine hoàn thành theo thứ tự bất định).
func (c *Checker) CheckAll(ctx context.Context, cams []camera.Camera) []Result {
	jobs := make(chan camera.Camera)
	results := make(chan Result)

	// 1) Khởi động các worker. WaitGroup đếm số goroutine còn sống.
	var wg sync.WaitGroup
	for i := 0; i < c.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cam := range jobs { // vòng lặp kết thúc khi jobs bị close
				results <- c.Check(ctx, cam)
			}
		}()
	}

	// 2) Producer: đẩy job vào channel; dừng sớm nếu ctx bị cancel.
	go func() {
		defer close(jobs) // close để workers thoát vòng range
		for _, cam := range cams {
			select {
			case jobs <- cam:
			case <-ctx.Done():
				return
			}
		}
	}()

	// 3) Khi tất cả worker xong thì đóng results để collector thoát vòng lặp.
	go func() {
		wg.Wait()
		close(results)
	}()

	// 4) Collector chạy ngay trên goroutine hiện tại.
	out := make([]Result, 0, len(cams))
	for r := range results {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b Result) int {
		return strings.Compare(a.CameraID, b.CameraID)
	})
	return out
}

// Watch kiểm tra định kỳ và đẩy từng lượt kết quả ra channel cho tới khi ctx bị cancel.
// Trả về channel chỉ-đọc (<-chan) để caller không thể vô tình ghi hay close.
func (c *Checker) Watch(ctx context.Context, cams []camera.Camera, interval time.Duration) <-chan []Result {
	out := make(chan []Result)
	go func() {
		defer close(out)
		ticker := time.NewTicker(interval)
		defer ticker.Stop() // luôn Stop ticker, nếu không nó tồn tại mãi (leak)

		for {
			results := c.CheckAll(ctx, cams)
			select {
			case out <- results:
			case <-ctx.Done():
				return
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// Summary tổng hợp một lượt kiểm tra.
type Summary struct {
	Total      int           `json:"total"`
	Online     int           `json:"online"`
	Offline    int           `json:"offline"`
	AvgLatency time.Duration `json:"avg_latency_ns"`
}

// Summarize tính Summary từ danh sách Result.
func Summarize(results []Result) Summary {
	var s Summary
	var totalLatency time.Duration
	for _, r := range results {
		s.Total++
		switch r.Status { // switch trong Go không fall-through, không cần break
		case camera.StatusOnline:
			s.Online++
			totalLatency += r.Latency
		case camera.StatusOffline:
			s.Offline++
		}
	}
	if s.Online > 0 {
		s.AvgLatency = totalLatency / time.Duration(s.Online) // ép kiểu tường minh, Go không auto-cast
	}
	return s
}

// String implement fmt.Stringer.
func (s Summary) String() string {
	return fmt.Sprintf("total=%d online=%d offline=%d avg_latency=%s",
		s.Total, s.Online, s.Offline, s.AvgLatency.Round(time.Millisecond))
}
