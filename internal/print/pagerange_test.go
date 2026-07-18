// internal/print/pagerange_test.go
package print

import (
	"reflect"
	"testing"
)

func TestParseRange_EmptyMeansAllPages(t *testing.T) {
	got, err := ParseRange("", 3)
	if err != nil {
		t.Fatalf("ParseRange: %v", err)
	}
	want := []int{0, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseRange(\"\", 3) = %v, want %v", got, want)
	}
}

func TestParseRange_CommaAndDash(t *testing.T) {
	got, err := ParseRange("1,3-5", 6)
	if err != nil {
		t.Fatalf("ParseRange: %v", err)
	}
	want := []int{0, 2, 3, 4} // 1-based "1,3-5" -> 0-based {0,2,3,4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseRange(\"1,3-5\", 6) = %v, want %v", got, want)
	}
}

func TestParseRange_DedupesAndSorts(t *testing.T) {
	got, err := ParseRange("3,1,2-3,1", 5)
	if err != nil {
		t.Fatalf("ParseRange: %v", err)
	}
	want := []int{0, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseRange(\"3,1,2-3,1\", 5) = %v, want %v", got, want)
	}
}

func TestParseRange_OutOfRangePagesSilentlyDropped(t *testing.T) {
	got, err := ParseRange("2,9999", 5)
	if err != nil {
		t.Fatalf("ParseRange: %v", err)
	}
	want := []int{1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseRange(\"2,9999\", 5) = %v, want %v", got, want)
	}
}

func TestParseRange_AllOutOfRangeIsError(t *testing.T) {
	_, err := ParseRange("9-12", 5)
	if err == nil {
		t.Fatalf("ParseRange(\"9-12\", 5) = nil error, want error (no valid pages)")
	}
}

func TestParseRange_InvalidTokenIsError(t *testing.T) {
	_, err := ParseRange("abc", 5)
	if err == nil {
		t.Fatalf("ParseRange(\"abc\", 5) = nil error, want error")
	}
}

func TestParseRange_ReversedRangeIsError(t *testing.T) {
	_, err := ParseRange("5-1", 5)
	if err == nil {
		t.Fatalf("ParseRange(\"5-1\", 5) = nil error, want error (start > end)")
	}
}

func TestParseRange_EmptyTokensSkipped(t *testing.T) {
	got, err := ParseRange("1,,3", 5)
	if err != nil {
		t.Fatalf("ParseRange: %v", err)
	}
	want := []int{0, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseRange(\"1,,3\", 5) = %v, want %v", got, want)
	}
}
