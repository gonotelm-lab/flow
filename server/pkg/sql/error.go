package sql

import (
	"gorm.io/gorm"

	pkgerr "github.com/gonotelm-lab/flow/server/pkg/errors"
	"github.com/pkg/errors"
)

func WrapError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return pkgerr.NoRecord
	}

	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return pkgerr.DuplicatedResource
	}

	return errors.WithStack(err)
}
