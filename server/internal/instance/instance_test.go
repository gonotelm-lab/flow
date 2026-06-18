package instance

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceEventType_String(t *testing.T) {
	assert.Equal(t, "PUT", InstanceEventPut.String())
	assert.Equal(t, "DELETE", InstanceEventDelete.String())
}

func TestNewInstance(t *testing.T) {
	const (
		createRevision int64         = 42
		expiry         time.Duration = 12 * time.Second
	)

	ins := NewInstance(createRevision, expiry)
	require.NotNil(t, ins)

	assert.Equal(t, InstanceGroup, ins.Group)
	assert.True(t, strings.HasPrefix(ins.Key, InstanceGroup+"/"))
	assert.NotEmpty(t, ins.Value)
	assert.Equal(t, createRevision, ins.CreateRevision)
	assert.NotZero(t, ins.FencingToken)
	assert.Greater(t, ins.ExpireTime, ins.StartTime)
}

func TestInstance_Duplicate(t *testing.T) {
	original := &Instance{
		Id:             7,
		Group:          InstanceGroup,
		Key:            "flow/instances/k-1",
		Value:          "v-1",
		StartTime:      100,
		ExpireTime:     200,
		FencingToken:   300,
		CreateRevision: 1,
	}

	dup := original.Duplicate()
	require.NotNil(t, dup)
	require.NotSame(t, original, dup)
	assert.Equal(t, *original, *dup)

	dup.Value = "changed"
	assert.Equal(t, "v-1", original.Value)
}

func TestInstance_TTLAndSchema(t *testing.T) {
	ins := &Instance{
		Id:             1,
		Group:          InstanceGroup,
		Key:            "k",
		Value:          "v",
		StartTime:      1_000,
		ExpireTime:     2_000,
		FencingToken:   3_000,
		CreateRevision: 4_000,
	}

	ins.ExtendTTL(3 * time.Second)
	assert.Equal(t, int64(5_000), ins.ExpireTime)

	ins.SetExpireTime(8_888)
	assert.Equal(t, int64(8_888), ins.ExpireTime)

	s := ins.ToSchema()
	require.NotNil(t, s)
	assert.Equal(t, ins.Id, s.Id)
	assert.Equal(t, ins.Group, s.Group)
	assert.Equal(t, ins.Key, s.Key)
	assert.Equal(t, ins.Value, s.Value)
	assert.Equal(t, ins.StartTime, s.StartTime)
	assert.Equal(t, ins.ExpireTime, s.ExpireTime)
	assert.Equal(t, ins.FencingToken, s.FencingToken)
	assert.Equal(t, ins.CreateRevision, s.CreateRevision)
}
