package pagination

import (
	"errors"
	"strconv"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPage         = 100_000
	maxPageSize     = 1_000
)

var ErrInvalidOffsetPagination = errors.New("invalid offset pagination")

func ParseOffset(pageValue, sizeValue string) (int, int, error) {
	page, err := parsePositive(pageValue, defaultPage)
	if err != nil || page > maxPage {
		return 0, 0, ErrInvalidOffsetPagination
	}
	size, err := parsePositive(sizeValue, defaultPageSize)
	if err != nil || size > maxPageSize {
		return 0, 0, ErrInvalidOffsetPagination
	}
	return page, size, nil
}

func parsePositive(value string, fallback int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return 0, ErrInvalidOffsetPagination
		}
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, ErrInvalidOffsetPagination
	}
	return parsed, nil
}
