package platform

import (
	"github.com/gin-gonic/gin"
	"strconv"
)

type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

func Pagination(c *gin.Context) (page, size int, err error) {
	page = 1
	size = 20
	var e error
	if v := c.Query("page"); v != "" {
		page, e = strconv.Atoi(v)
		if e != nil || page < 1 || page > 100000 {
			return 0, 0, Err(400, "invalid_pagination", "Page must be between 1 and 100000.")
		}
	}
	if v := c.Query("page_size"); v != "" {
		size, e = strconv.Atoi(v)
		if e != nil || size < 1 || size > 100 {
			return 0, 0, Err(400, "invalid_pagination", "Page size must be between 1 and 100.")
		}
	}
	return page, size, nil
}
