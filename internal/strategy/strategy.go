package strategy

import "strconv"

// Indicating is implemented by strategies that expose computed indicator series
// for charting. The map key is the legend label shown in the UI.
type Indicating interface {
	Indicators() map[string][]IndicatorPoint
}

func itoa(n int) string { return strconv.Itoa(n) }
