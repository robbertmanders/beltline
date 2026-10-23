package core

import (
	"fmt"
	"time"
)

// ScheduleDays is which days a schedule fires on.
type ScheduleDays string

const (
	// ScheduleDaysEveryDay fires every day.
	ScheduleDaysEveryDay ScheduleDays = "everyDay"
	// ScheduleDaysWeekdays fires Monday to Friday.
	ScheduleDaysWeekdays ScheduleDays = "weekdays"
	// ScheduleDaysWeekly fires once a week, on Schedule.Weekday.
	ScheduleDaysWeekly ScheduleDays = "weekly"
)

// AllScheduleDays lists every ScheduleDays in declaration order (CaseIterable).
var AllScheduleDays = []ScheduleDays{ScheduleDaysEveryDay, ScheduleDaysWeekdays, ScheduleDaysWeekly}

// Known reports whether x is one of the cases of ScheduleDays.
func (x ScheduleDays) Known() bool { return enumKnown(x, AllScheduleDays) }

// Label is the name shown for the days.
func (d ScheduleDays) Label() string {
	switch d {
	case ScheduleDaysEveryDay:
		return "Every day"
	case ScheduleDaysWeekdays:
		return "Monday to Friday"
	case ScheduleDaysWeekly:
		return "Once a week"
	}
	return ""
}

// Schedule is when a scheduled belt fires: at a time of day, on some days of the week.
type Schedule struct {
	Days ScheduleDays `json:"days"`
	// Weekday is for ScheduleDaysWeekly: 1 is Sunday, 2 Monday, through 7, Saturday.
	Weekday int `json:"weekday"`
	Hour    int `json:"hour"`
	Minute  int `json:"minute"`
}

// DefaultSchedule is the default schedule:
// weekdays, Monday, 09:00.
func DefaultSchedule() Schedule {
	return Schedule{Days: ScheduleDaysWeekdays, Weekday: 2, Hour: 9, Minute: 0}
}

// ScheduleWeekdayNames are the names of the weekdays, Sunday first.
var ScheduleWeekdayNames = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// Summary is one line about when it fires, such as "weekdays at 09:00".
func (s Schedule) Summary() string {
	clock := fmt.Sprintf("%02d:%02d", s.Hour, s.Minute)
	switch s.Days {
	case ScheduleDaysEveryDay:
		return "every day at " + clock
	case ScheduleDaysWeekdays:
		return "weekdays at " + clock
	case ScheduleDaysWeekly:
		return ScheduleWeekdayNames[max(1, min(s.Weekday, 7))-1] + "s at " + clock
	}
	return ""
}

// fires reports whether the schedule fires on day (the day's weekday in loc).
func (s Schedule) fires(day time.Time, loc *time.Location) bool {
	weekday := int(day.In(calendarLocation(loc)).Weekday()) + 1
	switch s.Days {
	case ScheduleDaysEveryDay:
		return true
	case ScheduleDaysWeekdays:
		return weekday >= 2 && weekday <= 6
	case ScheduleDaysWeekly:
		return weekday == s.Weekday
	}
	return false
}

// Latest is the latest time it fired at or before date, looking back a week, or
// nil.
func (s Schedule) Latest(date time.Time, loc *time.Location) *time.Time {
	loc = calendarLocation(loc)
	for back := 0; back <= 7; back++ {
		slot, ok := s.slotOn(date.In(loc).AddDate(0, 0, -back), loc)
		if ok && !slot.After(date) && s.fires(slot, loc) {
			return &slot
		}
	}
	return nil
}

// Next is the first time it fires after date, or nil.
func (s Schedule) Next(date time.Time, loc *time.Location) *time.Time {
	loc = calendarLocation(loc)
	for ahead := 0; ahead <= 8; ahead++ {
		slot, ok := s.slotOn(date.In(loc).AddDate(0, 0, ahead), loc)
		if ok && slot.After(date) && s.fires(slot, loc) {
			return &slot
		}
	}
	return nil
}

// slotOn is the time of the schedule's firing on the day of day, in loc, and
// whether the day has one. The wall time is set with the calendar's
// default matching policy, nextTime: an hour outside 0-23 or a minute
// outside 0-59 has no slot, so the day is skipped; a wall time inside a spring-
// forward gap gives the instant the clock jumps (02:30 becomes 03:00 in New
// York on 2026-03-08); a repeated wall time (fall-back 01:30) gives its first
// occurrence.
func (s Schedule) slotOn(day time.Time, loc *time.Location) (time.Time, bool) {
	if s.Hour < 0 || s.Hour > 23 || s.Minute < 0 || s.Minute > 59 {
		return time.Time{}, false
	}
	d := day.In(loc)
	wall := time.Date(d.Year(), d.Month(), d.Day(), s.Hour, s.Minute, 0, 0, loc)
	if !sameWallClock(wall, d.Year(), d.Month(), d.Day(), s.Hour, s.Minute) {
		return nextExistingWall(d.Year(), d.Month(), d.Day(), s.Hour, s.Minute, loc), true
	}
	return wall, true
}

// sameWallClock reports whether t reads as the given date and time in its own zone.
func sameWallClock(t time.Time, year int, month time.Month, day, hour, minute int) bool {
	return t.Year() == year && t.Month() == month && t.Day() == day && t.Hour() == hour && t.Minute() == minute
}

// nextExistingWall is the instant the clock jumps forward past the wall time
// asked for (the start of a spring-forward gap). Its UTC offset before the
// jump is o1 and after it o2 (o2 > o1); the jump X lies in (wall-o2, wall-o1],
// and a binary search over that interval finds the first instant whose offset
// is o2.
func nextExistingWall(year int, month time.Month, day, hour, minute int, loc *time.Location) time.Time {
	wall := time.Date(year, month, day, hour, minute, 0, 0, time.UTC).Unix()
	_, o1 := time.Unix(wall, 0).Add(-36 * time.Hour).In(loc).Zone()
	_, o2 := time.Unix(wall, 0).Add(36 * time.Hour).In(loc).Zone()
	lo := wall - int64(o2) // before the jump: offset o1
	hi := wall - int64(o1) // after the jump: offset o2
	for hi-lo > 1 {
		mid := lo + (hi-lo)/2
		if _, off := time.Unix(mid, 0).In(loc).Zone(); off == o2 {
			hi = mid
		} else {
			lo = mid
		}
	}
	return time.Unix(hi, 0).In(loc)
}

// calendarLocation is loc, or the machine's zone when loc is nil.
func calendarLocation(loc *time.Location) *time.Location {
	if loc == nil {
		return time.Local
	}
	return loc
}

// ScheduleKey is the task key a firing gets: its date, as 2026-09-29.
func ScheduleKey(slot time.Time, loc *time.Location) string {
	d := slot.In(calendarLocation(loc))
	return fmt.Sprintf("%04d-%02d-%02d", d.Year(), int(d.Month()), d.Day())
}
