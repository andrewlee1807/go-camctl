// Package camera chứa domain model của camera và các quy tắc validate.
//
// Quy ước Go: mỗi thư mục là một package, tên package = tên thư mục,
// và doc comment của package bắt đầu bằng "Package <tên>".
package camera

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Sentinel errors: các giá trị lỗi cố định để so sánh bằng errors.Is.
// Tương đương với việc Python định nghĩa các exception class riêng,
// nhưng ở Go lỗi chỉ là một giá trị (value), không phải luồng điều khiển.
var (
	ErrEmptyID    = errors.New("id is empty")
	ErrEmptyName  = errors.New("name is empty")
	ErrInvalidURL = errors.New("invalid rtsp url")
	ErrNotFound   = errors.New("camera not found")
)

// DefaultRTSPPort là cổng RTSP mặc định khi URL không ghi port.
const DefaultRTSPPort = "554"

// Status là trạng thái kết nối của camera.
// Go không có enum: quy ước là tạo một named type + hằng số iota.
type Status int

const (
	StatusUnknown Status = iota // 0
	StatusOnline                // 1
	StatusOffline               // 2
)

var statusNames = map[Status]string{
	StatusUnknown: "unknown",
	StatusOnline:  "online",
	StatusOffline: "offline",
}

// String implement interface fmt.Stringer, nên fmt.Println(StatusOnline) in ra "online".
// Tương đương __str__ trong Python.
func (s Status) String() string {
	if name, ok := statusNames[s]; ok {
		return name
	}
	return fmt.Sprintf("Status(%d)", int(s))
}

// MarshalText implement encoding.TextMarshaler: encoding/json sẽ tự dùng nó,
// nên JSON ghi ra "online" thay vì số 1.
func (s Status) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// ValidationError là lỗi có cấu trúc (mang thêm ngữ cảnh) và bọc (wrap) một lỗi gốc.
type ValidationError struct {
	CameraID string
	Field    string
	Err      error
}

// Error implement interface error. Dùng pointer receiver để errors.As
// khớp với kiểu *ValidationError một cách nhất quán.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("camera %q: field %s: %v", e.CameraID, e.Field, e.Err)
}

// Unwrap cho phép errors.Is(err, ErrEmptyName) "nhìn xuyên" qua ValidationError.
func (e *ValidationError) Unwrap() error { return e.Err }

// Camera là một camera trong farm. Struct tag `json:"..."` điều khiển encoding/json,
// giống Field(alias=...) trong pydantic.
type Camera struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	RTSPURL string   `json:"rtsp_url"`
	Zone    string   `json:"zone"`
	HasPTZ  bool     `json:"has_ptz"`
	Tags    []string `json:"tags,omitempty"`
}

// Normalize chuẩn hoá dữ liệu đầu vào. Dùng POINTER receiver vì hàm này SỬA struct;
// nếu dùng value receiver thì chỉ sửa trên bản copy và caller không thấy gì thay đổi.
func (c *Camera) Normalize() {
	c.ID = strings.TrimSpace(c.ID)
	c.Name = strings.TrimSpace(c.Name)
	c.Zone = strings.ToLower(strings.TrimSpace(c.Zone))
	for i, tag := range c.Tags {
		c.Tags[i] = strings.ToLower(strings.TrimSpace(tag))
	}
}

// Validate kiểm tra tính hợp lệ. Dùng VALUE receiver vì chỉ đọc, không sửa.
func (c Camera) Validate() error {
	if c.ID == "" {
		return &ValidationError{CameraID: c.ID, Field: "id", Err: ErrEmptyID}
	}
	if c.Name == "" {
		return &ValidationError{CameraID: c.ID, Field: "name", Err: ErrEmptyName}
	}
	if _, err := c.Addr(); err != nil {
		return &ValidationError{CameraID: c.ID, Field: "rtsp_url", Err: err}
	}
	return nil
}

// Addr trả về "host:port" để dial TCP tới camera.
// Go trả về nhiều giá trị (value, error) thay vì raise exception.
func (c Camera) Addr() (string, error) {
	u, err := url.Parse(c.RTSPURL)
	if err != nil {
		// %w bọc lỗi gốc để errors.Is/As còn tìm thấy nó; %v chỉ in chuỗi.
		return "", fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	if u.Scheme != "rtsp" && u.Scheme != "rtsps" {
		return "", fmt.Errorf("%w: scheme %q", ErrInvalidURL, u.Scheme)
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("%w: missing host", ErrInvalidURL)
	}
	port := u.Port()
	if port == "" {
		port = DefaultRTSPPort
	}
	return net.JoinHostPort(u.Hostname(), port), nil
}

// HasAnyTag trả về true nếu camera có ít nhất một trong các tag truyền vào.
// `tags ...string` là variadic param, tương đương *args trong Python.
func (c Camera) HasAnyTag(tags ...string) bool {
	for _, want := range tags {
		for _, have := range c.Tags {
			if have == want {
				return true
			}
		}
	}
	return false
}

// ValidateAll chuẩn hoá và validate cả danh sách, gom mọi lỗi lại bằng errors.Join.
// Lưu ý dùng `for i := range cams` rồi cams[i] để sửa phần tử thật;
// `for _, c := range cams` sẽ cho bạn một bản COPY của mỗi phần tử.
func ValidateAll(cams []Camera) error {
	var errs []error
	for i := range cams {
		cams[i].Normalize()
		if err := cams[i].Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...) // trả nil nếu errs rỗng
}
