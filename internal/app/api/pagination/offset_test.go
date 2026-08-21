package pagination

import (
	"errors"
	"testing"
)

func TestParseOffset(t *testing.T) {
	page, size, err := ParseOffset("", "")
	if err != nil || page != defaultPage || size != defaultPageSize {
		t.Fatalf("defaults = (%d, %d, %v)", page, size, err)
	}
	page, size, err = ParseOffset("2", "50")
	if err != nil || page != 2 || size != 50 {
		t.Fatalf("explicit values = (%d, %d, %v)", page, size, err)
	}
	page, size, err = ParseOffset("100000", "1000")
	if err != nil || page != maxPage || size != maxPageSize {
		t.Fatalf("maximum values = (%d, %d, %v)", page, size, err)
	}

	for _, values := range [][2]string{
		{"0", "20"},
		{"-1", "20"},
		{"+1", "20"},
		{" 1", "20"},
		{"invalid", "20"},
		{"1", "0"},
		{"1", "invalid"},
		{"100001", "20"},
		{"1", "1001"},
	} {
		if _, _, err := ParseOffset(values[0], values[1]); !errors.Is(err, ErrInvalidOffsetPagination) {
			t.Fatalf("ParseOffset(%q, %q) error = %v", values[0], values[1], err)
		}
	}
}
