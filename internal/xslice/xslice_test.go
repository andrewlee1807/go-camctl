package xslice

import (
	"strconv"
	"testing"
)

func TestMapFilterGroupBy(t *testing.T) {
	nums := []int{1, 2, 3, 4, 5}

	strs := Map(nums, strconv.Itoa)
	if len(strs) != 5 || strs[4] != "5" {
		t.Fatalf("Map: %v", strs)
	}

	even := Filter(nums, func(n int) bool { return n%2 == 0 })
	if len(even) != 2 || even[0] != 2 {
		t.Fatalf("Filter: %v", even)
	}

	groups := GroupBy(nums, func(n int) string {
		if n%2 == 0 {
			return "even"
		}
		return "odd"
	})
	if len(groups["odd"]) != 3 || len(groups["even"]) != 2 {
		t.Fatalf("GroupBy: %v", groups)
	}

	if got := Filter([]int{}, func(int) bool { return true }); got != nil {
		t.Fatalf("Filter on empty should return nil slice, got %v", got)
	}
}
