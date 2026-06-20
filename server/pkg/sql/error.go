package sql

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/pkg/errors"
)

var (
	ErrNoRecord      = fmt.Errorf("NO_RECORD")
	ErrDuplicatedKey = fmt.Errorf("DUPLICATED_KEY")
)

func WrapError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNoRecord
	}

	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicatedKey
	}

	return errors.WithStack(err)
}
