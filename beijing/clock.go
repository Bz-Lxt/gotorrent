package beijing

import "time"

var Zone = time.FixedZone("CST", 8*3600)

func Now() time.Time {
	return time.Now().In(Zone)
}

func Format(t time.Time) string {
	return t.In(Zone).Format("2006-01-02 15:04:05")
}

func Parse(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05", s, Zone)
}
