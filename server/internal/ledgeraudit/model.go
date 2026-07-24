package ledgeraudit

import "time"

const (
	KindCurrency = "currency"
	KindItem     = "item"
)

type UnbalancedEntry struct {
	Kind               string `json:"kind"`
	EntryID            int64  `json:"entryId"`
	PlayerID           string `json:"playerId"`
	Asset              string `json:"asset"`
	Amount             int64  `json:"amount"`
	CounterpartyAmount int64  `json:"counterpartyAmount"`
}

type Mismatch struct {
	Kind              string `json:"kind"`
	PlayerID          string `json:"playerId"`
	Asset             string `json:"asset"`
	Scope             string `json:"scope,omitempty"`
	ProjectionBalance int64  `json:"projectionBalance"`
	LedgerBalance     int64  `json:"ledgerBalance"`
}

type Report struct {
	CheckedAt                  time.Time         `json:"checkedAt"`
	Healthy                    bool              `json:"healthy"`
	CurrencyProjectionsChecked int64             `json:"currencyProjectionsChecked"`
	ItemProjectionsChecked     int64             `json:"itemProjectionsChecked"`
	LedgerEntriesChecked       int64             `json:"ledgerEntriesChecked"`
	UnbalancedEntries          []UnbalancedEntry `json:"unbalancedEntries"`
	Mismatches                 []Mismatch        `json:"mismatches"`
}

func (r *Report) finalize() {
	r.Healthy = len(r.UnbalancedEntries) == 0 && len(r.Mismatches) == 0
}
