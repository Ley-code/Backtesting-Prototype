package strategy

import "strconv"

type Indicating interface {
	Indicators() map[string][]IndicatorPoint
}

func itoa(n int) string { return strconv.Itoa(n) }
