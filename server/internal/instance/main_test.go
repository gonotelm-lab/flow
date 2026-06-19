package instance

import (
	"context"
	"os"
	"testing"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/internal/repository/tx"
	pkgsql "github.com/gonotelm-lab/flow/server/pkg/sql"
	"gorm.io/gorm"
)

var gTestTxCtx context.Context

const testInstanceGroup = "flow/instances"

func TestMain(m *testing.M) {
	gTestTxCtx = tx.WithTTx(context.Background(), &gorm.DB{})
	os.Exit(m.Run())
}

func testTxContext() context.Context {
	if gTestTxCtx == nil {
		return tx.WithTTx(context.Background(), &gorm.DB{})
	}
	return gTestTxCtx
}

type fakeInstanceStore struct {
	createFn     func(ctx context.Context, instance *schema.Instance) (*schema.Instance, error)
	deleteFn     func(ctx context.Context, id int64) error
	getFn        func(ctx context.Context, id int64) (*schema.Instance, error)
	listActiveFn func(ctx context.Context, aliveAfterMs int64) ([]*schema.Instance, error)

	updateExpireTimeFn func(ctx context.Context, id int64, expireTimeMs, expectToken int64) (bool, error)
	listExpiredFn      func(ctx context.Context, expireBeforeMs int64, limit int) ([]*schema.Instance, error)
	deleteExpiredFn    func(ctx context.Context, id int64, expireBeforeMs int64) (bool, error)
}

func (f *fakeInstanceStore) Create(
	ctx context.Context,
	instance *schema.Instance,
) (*schema.Instance, error) {
	if f.createFn != nil {
		return f.createFn(ctx, instance)
	}
	return instance, nil
}

func (f *fakeInstanceStore) Delete(ctx context.Context, id int64) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, id)
	}
	return nil
}

func (f *fakeInstanceStore) Get(ctx context.Context, id int64) (*schema.Instance, error) {
	if f.getFn != nil {
		return f.getFn(ctx, id)
	}
	return nil, pkgsql.ErrNoRecord
}

func (f *fakeInstanceStore) ListActive(
	ctx context.Context,
	aliveAfterMs int64,
) ([]*schema.Instance, error) {
	if f.listActiveFn != nil {
		return f.listActiveFn(ctx, aliveAfterMs)
	}
	return nil, nil
}

func (f *fakeInstanceStore) UpdateExpireTime(
	ctx context.Context,
	id int64,
	expireTimeMs int64,
	expectToken int64,
) (bool, error) {
	if f.updateExpireTimeFn != nil {
		return f.updateExpireTimeFn(ctx, id, expireTimeMs, expectToken)
	}
	return true, nil
}

func (f *fakeInstanceStore) ListExpired(
	ctx context.Context,
	expireBeforeMs int64,
	limit int,
) ([]*schema.Instance, error) {
	if f.listExpiredFn != nil {
		return f.listExpiredFn(ctx, expireBeforeMs, limit)
	}
	return nil, nil
}

func (f *fakeInstanceStore) DeleteExpired(
	ctx context.Context,
	id int64,
	expireBeforeMs int64,
) (bool, error) {
	if f.deleteExpiredFn != nil {
		return f.deleteExpiredFn(ctx, id, expireBeforeMs)
	}
	return true, nil
}

type fakeInstanceEventStore struct {
	appendFn func(ctx context.Context, event *schema.InstanceEvent) error
	lastFn   func(ctx context.Context, group string) (*schema.InstanceEvent, error)
	listFn   func(ctx context.Context, group string, lastRevision int64, limit int) ([]*schema.InstanceEvent, error)
}

func (f *fakeInstanceEventStore) Append(ctx context.Context, event *schema.InstanceEvent) error {
	if f.appendFn != nil {
		return f.appendFn(ctx, event)
	}
	return nil
}

func (f *fakeInstanceEventStore) Last(ctx context.Context, group string) (*schema.InstanceEvent, error) {
	if f.lastFn != nil {
		return f.lastFn(ctx, group)
	}
	return nil, pkgsql.ErrNoRecord
}

func (f *fakeInstanceEventStore) List(
	ctx context.Context,
	group string,
	lastRevision int64,
	limit int,
) ([]*schema.InstanceEvent, error) {
	if f.listFn != nil {
		return f.listFn(ctx, group, lastRevision, limit)
	}
	return nil, nil
}

type fakeGlobalRevisionStore struct {
	getOrInitForUpdateFn func(ctx context.Context, zero *schema.GlobalRevision) (*schema.GlobalRevision, error)
	incrRevisionFn       func(ctx context.Context, name string, updateTime int64) error
	getFn                func(ctx context.Context, name string) (*schema.GlobalRevision, error)
}

func (f *fakeGlobalRevisionStore) GetOrInitForUpdate(
	ctx context.Context,
	zero *schema.GlobalRevision,
) (*schema.GlobalRevision, error) {
	if f.getOrInitForUpdateFn != nil {
		return f.getOrInitForUpdateFn(ctx, zero)
	}
	return &schema.GlobalRevision{Name: zero.Name, CurrentRevision: 0, UpdateTime: zero.UpdateTime}, nil
}

func (f *fakeGlobalRevisionStore) IncrRevision(
	ctx context.Context,
	name string,
	updateTime int64,
) error {
	if f.incrRevisionFn != nil {
		return f.incrRevisionFn(ctx, name, updateTime)
	}
	return nil
}

func (f *fakeGlobalRevisionStore) Get(
	ctx context.Context,
	name string,
) (*schema.GlobalRevision, error) {
	if f.getFn != nil {
		return f.getFn(ctx, name)
	}
	return nil, pkgsql.ErrNoRecord
}
