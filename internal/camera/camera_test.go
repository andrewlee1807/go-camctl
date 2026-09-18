package camera

import (
	"encoding/json"
	"errors"
	"testing"
)

// Table-driven test: idiom chuẩn của Go, tương đương pytest.mark.parametrize.
func TestCamera_Validate(t *testing.T) {
	valid := Camera{ID: "cam-1", Name: "Gate", RTSPURL: "rtsp://admin:pw@10.0.0.5/stream1"}

	tests := []struct {
		name    string
		mutate  func(c *Camera) // closure sửa camera hợp lệ thành ca test
		wantErr error           // nil = mong đợi hợp lệ
	}{
		{name: "valid", mutate: func(c *Camera) {}, wantErr: nil},
		{name: "empty id", mutate: func(c *Camera) { c.ID = "" }, wantErr: ErrEmptyID},
		{name: "empty name", mutate: func(c *Camera) { c.Name = "" }, wantErr: ErrEmptyName},
		{name: "http scheme", mutate: func(c *Camera) { c.RTSPURL = "http://x" }, wantErr: ErrInvalidURL},
		{name: "no host", mutate: func(c *Camera) { c.RTSPURL = "rtsp:///stream" }, wantErr: ErrInvalidURL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { // subtest: go test -run 'TestCamera_Validate/empty_id'
			c := valid // struct là value type → đây là bản copy, không ảnh hưởng ca khác
			tt.mutate(&c)

			err := c.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("expected *ValidationError, got %T", err)
			}
		})
	}
}

func TestCamera_Addr(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"rtsp://10.0.0.5/stream", "10.0.0.5:554"},
		{"rtsp://admin:pw@cam.local:8554/live", "cam.local:8554"},
		{"rtsps://[::1]:322/x", "[::1]:322"},
	}
	for _, tt := range tests {
		got, err := Camera{RTSPURL: tt.url}.Addr()
		if err != nil {
			t.Errorf("%s: unexpected error %v", tt.url, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s: got %q want %q", tt.url, got, tt.want)
		}
	}
}

func TestCamera_Normalize(t *testing.T) {
	c := Camera{ID: " cam-1 ", Name: " Gate ", Zone: " North ", Tags: []string{" PTZ ", "Gate"}}
	c.Normalize()
	if c.ID != "cam-1" || c.Name != "Gate" || c.Zone != "north" {
		t.Fatalf("normalize failed: %+v", c)
	}
	if c.Tags[0] != "ptz" || c.Tags[1] != "gate" {
		t.Fatalf("tags not normalized: %v", c.Tags)
	}
}

func TestStatus_JSON(t *testing.T) {
	data, err := json.Marshal(map[string]Status{"s": StatusOnline})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != `{"s":"online"}` {
		t.Fatalf("got %s", got)
	}
	if Status(99).String() != "Status(99)" {
		t.Fatal("unknown status should print numeric form")
	}
}

func TestValidateAll(t *testing.T) {
	cams := []Camera{
		{ID: "a", Name: "A", RTSPURL: "rtsp://h/1"},
		{ID: "", Name: "B", RTSPURL: "rtsp://h/2"},
		{ID: "c", Name: "", RTSPURL: "rtsp://h/3"},
	}
	err := ValidateAll(cams)
	if !errors.Is(err, ErrEmptyID) || !errors.Is(err, ErrEmptyName) {
		t.Fatalf("joined error should contain both sentinels, got: %v", err)
	}
}
