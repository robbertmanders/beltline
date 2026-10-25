package core

import (
	"math"
	"sort"
	"strconv"
	"time"
)

// overviewSummaryStartOfDay is the midnight that starts now's day in loc.
func overviewSummaryStartOfDay(now time.Time, loc *time.Location) time.Time {
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// localDay is one calendar day in a location (Calendar.isDate(_:inSameDayAs:)), for checking many times against the same day
// without working out that day's date again for each. after and before bound the instants the day can
// hold, so a time far from the day, as most of a run book's are, is told apart without a date.
type localDay struct {
	year          int
	month         time.Month
	day           int
	loc           *time.Location
	after, before time.Time
}

// localDayMargin is how far past its midnights, as time.Date reads them, a day may still reach. A zone's
// change moves its midnight by hours, and at most by a day (Samoa skipped 30 December 2011), so two
// days are more than enough.
const localDayMargin = 48 * time.Hour

// localDayOf is the day t falls on in loc.
func localDayOf(t time.Time, loc *time.Location) localDay {
	year, month, day := t.In(loc).Date()
	return localDay{
		year: year, month: month, day: day, loc: loc,
		after:  time.Date(year, month, day, 0, 0, 0, 0, loc).Add(-localDayMargin),
		before: time.Date(year, month, day+1, 0, 0, 0, 0, loc).Add(localDayMargin),
	}
}

// holds reports whether t falls on the day, in the day's location.
func (d localDay) holds(t time.Time) bool {
	if t.Before(d.after) || !t.Before(d.before) {
		return false
	}
	year, month, day := t.In(d.loc).Date()
	return year == d.year && month == d.month && day == d.day
}

// OverviewSummaryShade is a block's shade: none for zero, otherwise one of four steps up to the
// busiest block.
func OverviewSummaryShade(value, most int) int {
	if value <= 0 {
		return 0
	}
	denominator := float64(max(most, 1))
	step := int(math.Ceil(float64(OverviewSummaryShadeSteps) * float64(value) / denominator))
	return min(OverviewSummaryShadeSteps, max(1, step))
}

// overviewSummaryAxisDate is "SEP 27" style axis text for a day, in en_US_POSIX "MMM d".
// The formatter uses the system time zone, so the day is read in time.Local.
func overviewSummaryAxisDate(date time.Time) string {
	local := date.In(time.Local)
	return local.Month().String()[:3] + " " + strconv.Itoa(local.Day())
}

// OverviewSummaryChartFor is the shipped sparkline's labels: the ISO weeks its twelve weeks span, and the latest
// week's count.
func OverviewSummaryChartFor(weeks []int, now time.Time) OverviewSummaryChart {
	_, week := now.In(time.Local).ISOWeek()
	return OverviewSummaryChart{
		StartLabel:  overviewSummaryStartPrefix + strconv.Itoa((week+40)%52+1),
		EndLabel:    overviewSummaryStartPrefix + strconv.Itoa(week),
		LatestLabel: "[ " + strconv.Itoa(weeks[len(weeks)-1]) + " ]",
	}
}

// OverviewSummaryGreeting is the greeting for the daemon's clock, in the local hours the
// Overview's header uses.
func OverviewSummaryGreeting(now time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	hour := now.In(loc).Hour()
	switch {
	case hour < 12:
		return "Good morning."
	case hour < 18:
		return "Good afternoon."
	}
	return "Good evening."
}

// OverviewSummaryBusyCaption is "N AGENTS BUSY", the Running post's foot caption.
func OverviewSummaryBusyCaption(running int) string {
	return strconv.Itoa(running) + " AGENTS BUSY"
}

// overviewSummaryMedian is the median of times, each a shipped run's seconds from creation to update.
// It sorts times in place.
func overviewSummaryMedian(times []float64) string {
	if len(times) == 0 {
		return "—"
	}
	sort.Float64s(times)
	minutes := int(times[len(times)/2] / 60)
	if minutes >= 60 {
		return strconv.Itoa(minutes/60) + " H " + strconv.Itoa(minutes%60) + overviewSummaryMinutesUnit
	}
	return strconv.Itoa(minutes) + overviewSummaryMinutesUnit
}
