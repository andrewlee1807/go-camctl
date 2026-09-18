# camctl — Đề bài & lời giải mẫu

Mini-project để luyện toàn bộ cú pháp cơ bản của Go, đặt trong bối cảnh dự án camera farm.
Chỉ dùng thư viện chuẩn (stdlib), không cần dependency ngoài.

## 1. Đề bài

Viết công cụ dòng lệnh `camctl` quản lý **danh mục camera** của một farm và **kiểm tra tình trạng kết nối RTSP** của chúng.

### Dữ liệu
Registry là một file JSON chứa mảng camera:

```json
{ "id": "gate-01", "name": "Cổng chính", "rtsp_url": "rtsp://admin:admin@10.0.0.5:554/stream1",
  "zone": "north", "has_ptz": true, "tags": ["ptz", "entrance"] }
```

Quy tắc hợp lệ: `id`, `name` không rỗng; `rtsp_url` phải có scheme `rtsp`/`rtsps` và có host; nếu thiếu port thì mặc định 554. Trước khi lưu phải chuẩn hoá: trim khoảng trắng, `zone` và `tags` về chữ thường.

### Lệnh cần có
| Lệnh | Yêu cầu |
|---|---|
| `list [-zone Z] [-json]` | In bảng camera (sort theo id) hoặc JSON |
| `add -id -name -url [-zone] [-ptz] [-tags a,b]` | Thêm/ghi đè camera, validate, lưu file **atomic** (ghi tạm rồi rename) |
| `remove -id` | Xoá; báo lỗi rõ nếu không tồn tại |
| `check [-timeout] [-workers N] [-zone] [-json]` | Mở TCP tới `host:port` của **mọi camera song song** (tối đa N cùng lúc), báo online/offline + latency, in tổng kết |
| `watch [-interval]` | Lặp `check` định kỳ, dừng sạch khi Ctrl+C |
| `stats` | Nhóm theo zone: total, online, uptime %, latency trung bình |

### Yêu cầu kỹ thuật
1. Layout chuẩn: `cmd/camctl/main.go` + `internal/<package>`; `main()` mỏng, logic trong `run()`.
2. Lỗi: sentinel error (`ErrNotFound`, `ErrInvalidURL`…), một custom error type có `Unwrap`, dùng `%w` để bọc, `errors.Is/As/Join` để kiểm tra. Exit code: 0 ok, 1 lỗi chung, 2 input sai, 130 bị cancel.
3. `Store` là **interface** với 2 implementation: in-memory (an toàn concurrency) và file JSON **tái sử dụng** in-memory qua embedding.
4. Health check nhận `Dialer` qua interface để test không cần mạng; cấu hình bằng functional options.
5. Worker pool bằng goroutine + channel + `sync.WaitGroup`; mọi I/O nhận `context.Context`; tôn trọng cancel.
6. `Status` là enum (`iota`) có `String()` và marshal JSON ra chữ.
7. Có ít nhất một hàm generic (`Map/Filter/GroupBy`).
8. Test table-driven, contract test cho Store, test concurrency chạy được với `go test -race ./...`.
9. `gofmt` sạch, `go vet` sạch.

### Checklist cú pháp được dùng → ở đâu
| Cú pháp | Vị trí |
|---|---|
| package, import, exported/unexported, `internal/` | mọi file |
| const, `iota`, named type, Stringer | `camera/camera.go` |
| struct, struct tag, value vs pointer receiver | `camera/camera.go` (`Validate` vs `Normalize`) |
| multiple return, `error`, `%w`, `errors.Is/As/Join`, custom error + `Unwrap` | `camera/camera.go`, `main.go: exitCode, errText` |
| slice, `make`, `append`, `for range`, sửa phần tử qua index | `camera.ValidateAll`, `xslice` |
| map, comma-ok, `delete`, duyệt map không có thứ tự | `store/store.go`, `main.go: runStats` |
| interface ngầm định, compile-time check `var _ I = (*T)(nil)` | `store/store.go`, `health/checker.go` |
| struct embedding (composition thay inheritance) | `store.File`, `health_test: nopConn` |
| closure, functional options, variadic `...` | `health.Option`, `camera.HasAnyTag`, `xslice` |
| generics `[T any]`, `comparable` | `xslice/xslice.go` |
| goroutine, channel, `select`, `close`, `<-chan`, WaitGroup, Mutex/RWMutex, atomic | `health/checker.go`, `store.Memory`, tests |
| `context` timeout/cancel, `signal.NotifyContext` | `health.Check`, `main.run` |
| `defer` | khắp nơi (Unlock, cancel, Close, ticker.Stop) |
| `switch` giá trị, `switch {}` điều kiện, type switch `v.(type)` | `health.Summarize`, `main.exitCode`, `main.formatCell` |
| `encoding/json` Marshal/Unmarshal, `MarshalText` | `store.File`, `camera.Status`, `main.writeJSON` |
| `flag.FlagSet` subcommand, `os.Args`, `os.Exit` | `main.go` |
| `time.Duration`, `Ticker`, `time.Since` | `health/checker.go` |
| `text/tabwriter`, `io.Writer` để test được output | `main.go` |
| `testing`: table-driven, `t.Run`, `t.TempDir`, external test package, fake qua interface | `*_test.go` |

## 2. Chạy thử

```bash
go vet ./... && go test -race ./...
go build -o camctl ./cmd/camctl

./camctl list  -file testdata/cameras.json
./camctl check -file testdata/cameras.json -timeout 1s -workers 4
./camctl stats -file testdata/cameras.json -json
./camctl watch -file testdata/cameras.json -interval 5s      # Ctrl+C để dừng
./camctl add   -file my.json -id cam-1 -name "Cổng" -url rtsp://admin:pw@10.0.0.5/live -ptz -tags ptz,gate
```

Muốn thấy camera **online**: chạy MediaMTX local (`docker run --rm -p 8554:8554 bluenviron/mediamtx`) — hai camera đầu trong `testdata/cameras.json` trỏ về `127.0.0.1:8554`.

## 3. Cấu trúc

```
camctl/
├── go.mod
├── cmd/camctl/main.go        # CLI: subcommand, flag, exit code, output
├── internal/
│   ├── camera/               # domain model, validate, errors, Status enum
│   ├── store/                # Store interface, Memory, File (embedding)
│   ├── health/               # Dialer interface, Checker, worker pool, Watch
│   └── xslice/               # generic Map/Filter/GroupBy
└── testdata/cameras.json
```

## 4. Bài tập mở rộng (tự làm)
- Thêm lệnh `discover` dùng WS-Discovery (UDP multicast 3702) tìm camera ONVIF trong LAN.
- Thay `Store` bằng SQLite/Postgres (chỉ cần thêm một type mới thoả interface — chạy lại `testStore`).
- `check` gửi hẳn request RTSP `OPTIONS` và đọc dòng `RTSP/1.0 200 OK` thay vì chỉ mở TCP.
- Biến `watch` thành HTTP server `GET /health` trả JSON lượt kiểm tra mới nhất (bước đệm sang web service).
