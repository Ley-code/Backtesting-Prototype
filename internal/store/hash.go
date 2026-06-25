package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

type HashableRequest struct {
	Symbol   string         `json:"symbol"`
	Interval string         `json:"interval"`
	Strategy string         `json:"strategy"`
	Days     int            `json:"days"`
	Params   map[string]int `json:"params"`
}

func RequestHash(symbol, interval, strategy string, days int, params map[string]int) (string, error) {
	p := make(map[string]int, len(params))
	for k, v := range params {
		p[k] = v
	}
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]int, len(keys))
	for _, k := range keys {
		ordered[k] = p[k]
	}
	body, err := json.Marshal(HashableRequest{
		Symbol: symbol, Interval: interval, Strategy: strategy,
		Days: days, Params: ordered,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
