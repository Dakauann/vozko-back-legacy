package conversation_repository

import (
	"fmt"

	"gorm.io/gorm"
)

const pageTotalColumn = "COUNT(*) OVER () AS page_total"

type countedPage struct {
	countSQL  string
	countArgs []interface{}
	pageSQL   string
	pageArgs  []interface{}
	offset    int
}

type PageTotal struct {
	Total int64 `gorm:"column:page_total"`
}

func (p PageTotal) pageTotal() int64 {
	return p.Total
}

type totalledRow interface {
	pageTotal() int64
}

func readCountedPage[Row totalledRow](db *gorm.DB, page countedPage) ([]Row, int64, error) {
	var rows []Row
	if err := db.Raw(page.pageSQL, page.pageArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	if len(rows) > 0 {
		return rows, rows[0].pageTotal(), nil
	}
	if page.offset <= 0 {
		return nil, 0, nil
	}
	var total int64
	if err := db.Raw(page.countSQL, page.countArgs...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("counting past the last page: %w", err)
	}
	return nil, total, nil
}
