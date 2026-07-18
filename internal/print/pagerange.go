// internal/print/pagerange.go
package print

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ParseRange parses a 1-based page-range spec like "1,8,9-12" (the
// syntax shown to the user in the print dialog's "选择页" box) into a
// sorted, de-duplicated list of 0-based page indices within
// [0, pageCount). An empty spec means "all pages".
//
// Page numbers outside [1, pageCount] are silently dropped rather than
// treated as an error - different files in a batch have different page
// counts, so referencing a page that doesn't exist in THIS file is
// expected, not a mistake. But if parsing yields zero pages at all -
// either every number was out of range, or a malformed token (not a
// number, not a valid "N-M" range, or a range with start > end) -
// ParseRange returns an error, so the caller can record the whole file
// as failed with reason "页码范围无效" rather than silently print nothing.
func ParseRange(spec string, pageCount int) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		if pageCount <= 0 {
			return nil, fmt.Errorf("print: page count must be positive, got %d", pageCount)
		}
		pages := make([]int, pageCount)
		for i := range pages {
			pages[i] = i
		}
		return pages, nil
	}

	seen := make(map[int]bool)
	var pages []int
	add := func(n int) {
		if n < 1 || n > pageCount {
			return
		}
		idx := n - 1
		if !seen[idx] {
			seen[idx] = true
			pages = append(pages, idx)
		}
	}

	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if dash := strings.IndexByte(part, '-'); dash > 0 {
			start, err := strconv.Atoi(strings.TrimSpace(part[:dash]))
			if err != nil {
				return nil, fmt.Errorf("print: invalid range %q", part)
			}
			end, err := strconv.Atoi(strings.TrimSpace(part[dash+1:]))
			if err != nil {
				return nil, fmt.Errorf("print: invalid range %q", part)
			}
			if start > end {
				return nil, fmt.Errorf("print: invalid range %q (start > end)", part)
			}
			for n := start; n <= end && n <= pageCount; n++ {
				add(n)
			}
			continue
		}

		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("print: invalid page number %q", part)
		}
		add(n)
	}

	sort.Ints(pages)

	if len(pages) == 0 {
		return nil, fmt.Errorf("print: no valid pages in range %q for a %d-page document", spec, pageCount)
	}
	return pages, nil
}
