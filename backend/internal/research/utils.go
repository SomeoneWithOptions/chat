package research

import "unicode/utf8"

func trimToRunes(raw string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(raw) <= limit {
		return raw
	}
	return string([]rune(raw)[:limit])
}
