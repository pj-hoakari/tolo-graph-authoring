package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type jsonColumn[T any] struct {
	value T
}

func (c jsonColumn[T]) Value() (driver.Value, error) {
	encoded, err := json.Marshal(c.value)
	if err != nil {
		return nil, err
	}

	return string(encoded), nil
}

func (c *jsonColumn[T]) Scan(src any) error {
	switch src := src.(type) {
	case []byte:
		return json.Unmarshal(src, &c.value)
	case string:
		return json.Unmarshal([]byte(src), &c.value)
	default:
		return fmt.Errorf("scan JSON column: unsupported type %T", src)
	}
}
