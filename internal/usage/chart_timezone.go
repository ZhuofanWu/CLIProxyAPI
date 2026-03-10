package usage

import "time"

const (
	usageChartTimeZoneName         = "Asia/Shanghai"
	usageChartSQLiteOffsetModifier = "+8 hours"
)

var usageChartLoc = func() *time.Location {
	loc, err := time.LoadLocation(usageChartTimeZoneName)
	if err != nil {
		return time.FixedZone("UTC+8", 8*60*60)
	}
	return loc
}()

func usageChartLocation() *time.Location {
	return usageChartLoc
}

func usageChartLocalTime(value time.Time) time.Time {
	return value.In(usageChartLocation())
}

func usageChartStartOfHour(value time.Time) time.Time {
	localValue := usageChartLocalTime(value)
	return time.Date(
		localValue.Year(),
		localValue.Month(),
		localValue.Day(),
		localValue.Hour(),
		0,
		0,
		0,
		usageChartLocation(),
	).UTC()
}

func usageChartStartOfDay(value time.Time) time.Time {
	localValue := usageChartLocalTime(value)
	return time.Date(
		localValue.Year(),
		localValue.Month(),
		localValue.Day(),
		0,
		0,
		0,
		0,
		usageChartLocation(),
	).UTC()
}

func usageRecordBucketGroupExpr(granularity string) string {
	if granularity == trendGranularityHour || granularity == tokenBreakdownGranularityHour {
		return "strftime('%Y-%m-%dT%H:00:00', timestamp_utc, '" + usageChartSQLiteOffsetModifier + "')"
	}
	return "date(timestamp_utc, '" + usageChartSQLiteOffsetModifier + "')"
}
