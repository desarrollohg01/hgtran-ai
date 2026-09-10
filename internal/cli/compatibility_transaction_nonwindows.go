//go:build !windows

package cli

import "github.com/desarrollohg01/hgtran-ai/v2/internal/model"

func usesAnchoredCompatibilityTransaction() bool {
	return false
}

func newCompatibilityRefreshTransaction(string, []model.ComponentID, model.Selection) (compatibilityRefreshTransaction, error) {
	return nil, nil
}
