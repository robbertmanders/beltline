// The durations and clock a run shows. The status types are in present_status.go.

package core

import (
	"fmt"
	"math"
	"strconv"
)

// StatusDuration formats seconds as "Xh Ym", "Ym Zs" or "Zs".
func StatusDuration(seconds float64) string {
	total := max(0, int(math.Round(seconds)))
	m := total / 60
	if m >= 60 {
		return strconv.Itoa(m/60) + "h " + strconv.Itoa(m%60) + "m"
	}
	if m > 0 {
		return strconv.Itoa(m) + "m " + strconv.Itoa(total%60) + "s"
	}
	return strconv.Itoa(total) + "s"
}

// StatusClock formats seconds as "MM:SS".
func StatusClock(seconds float64) string {
	total := max(0, int(seconds))
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}
