package googlesheets

import (
	"context"
	"fmt"

	"vozko/domain/googlesheet"
)

func (c *Client) DeleteRow(ctx context.Context, token string, target googlesheet.Target, row int) error {
	start := row - 1
	if start < firstBodyIndex {
		return fmt.Errorf("googlesheets: the header is never deleted, refused row %d", row)
	}
	return c.batchUpdate(ctx, token, target.SpreadsheetID, []map[string]any{
		{"deleteDimension": map[string]any{
			"range": map[string]any{"sheetId": target.TabID, "dimension": "ROWS", "startIndex": start, "endIndex": start + 1},
		}},
	})
}
