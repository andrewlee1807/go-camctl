// Package xslice cung cấp vài hàm generic tiện dụng trên slice.
//
// Đây là chỗ minh hoạ generics (Go 1.18+). Trong Python bạn có sẵn
// list comprehension / itertools.groupby; Go không có nên viết một lần dùng nhiều.
package xslice

// Map biến đổi []T thành []U. `[T, U any]` là type parameter, `any` = interface{}.
func Map[T, U any](in []T, f func(T) U) []U {
	out := make([]U, 0, len(in)) // pre-allocate capacity để tránh grow nhiều lần
	for _, v := range in {
		out = append(out, f(v))
	}
	return out
}

// Filter giữ lại các phần tử mà keep(v) == true.
func Filter[T any](in []T, keep func(T) bool) []T {
	var out []T // nil slice, append vào nil slice là hợp lệ
	for _, v := range in {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

// GroupBy gom các phần tử theo khoá. K phải `comparable` vì được dùng làm key của map.
func GroupBy[K comparable, T any](in []T, key func(T) K) map[K][]T {
	out := make(map[K][]T)
	for _, v := range in {
		k := key(v)
		out[k] = append(out[k], v) // đọc key chưa có trong map trả về zero value (nil slice)
	}
	return out
}
