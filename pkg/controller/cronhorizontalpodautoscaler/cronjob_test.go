package cronhorizontalpodautoscaler

import (
	"testing"
	"time"
	"fmt"
)

var (
	now     = time.Now()
	day     = now.Day()
	month   = now.Month()
	weekday = now.Weekday()
)

func TestSingleExcludeDates(t *testing.T) {
	for _, dataset := range createValidTestDataSet() {
		if skip, _ := IsTodayOff(dataset); !skip {
			t.Fatalf("valid dataset %v should pass.", dataset)
		}
	}

	for _, dataset := range createInvalidTestDataSet() {
		if skip, _ := IsTodayOff(dataset); skip {
			t.Fatalf("invalid dataset %v should not pass.", dataset)
		}
	}
	t.Log("all dataset pass")
}

func createValidTestDataSet() [][]string {
	return [][]string{
		[]string{
			fmt.Sprintf("* * * %d * *", day),
		},
		[]string{
			fmt.Sprintf("* * * * %d *", month),
		},
		[]string{
			fmt.Sprintf("* * * * * %d", weekday),
		},
		[]string{
			"* * * 1-31 * *",
		},
		[]string{
			"* * * * 1-12 *",
		},
		[]string{
			"* * * * * 0-6",
		},
	}
}

func createInvalidTestDataSet() [][]string {
	return [][]string{
		[]string{
			fmt.Sprintf("* * * %d * *", (day)%31+1),
		},
		[]string{
			fmt.Sprintf("* * * * %d *", (month)%12+1),
		},
		[]string{
			fmt.Sprintf("* * * * * %d", (weekday)%6+1),
		},
	}
}
