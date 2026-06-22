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

	ins := NewInstance(testInstanceGroup, "v", createRevision, expiry)
	require.NotNil(t, ins)

	assert.Equal(t, testInstanceGroup, ins.group)
	assert.True(t, strings.HasPrefix(ins.key, testInstanceGroup+"/"))
	assert.NotEmpty(t, ins.value)
	assert.Equal(t, createRevision, ins.createRevision)
	assert.NotZero(t, ins.fencingToken)
	assert.Greater(t, ins.expireTime, ins.startTime)
}

func TestNewInstance_CustomGroup(t *testing.T) {
	const (
		customGroup    = "custom/instances"
		createRevision = int64(7)
		expiry         = 2 * time.Second
	)

	ins := NewInstance(customGroup, "v", createRevision, expiry)
	require.NotNil(t, ins)
	assert.Equal(t, customGroup, ins.group)
	assert.True(t, strings.HasPrefix(ins.key, customGroup+"/"))
}

func TestNewInstance_TrimGroup(t *testing.T) {
	ins := NewInstance("  custom/instances  ", "v", 1, time.Second)
	require.NotNil(t, ins)
	assert.Equal(t, "custom/instances", ins.group)
	assert.True(t, strings.HasPrefix(ins.key, "custom/instances/"))
}

func TestInstance_TTLAndSchema(t *testing.T) {
	ins := &Instance{
		id:             1,
		group:          testInstanceGroup,
		key:            "k",
		value:          "v",
		startTime:      1_000,
		expireTime:     2_000,
		fencingToken:   3_000,
		createRevision: 4_000,
	}

	ins.SetExpireTime(8_888)
	assert.Equal(t, int64(8_888), ins.expireTime)

	s := ins.ToSchema()
	require.NotNil(t, s)
	assert.Equal(t, ins.id, s.Id)
	assert.Equal(t, ins.group, s.Group)
	assert.Equal(t, ins.key, s.Key)
	assert.Equal(t, ins.value, s.Value)
	assert.Equal(t, ins.startTime, s.StartTime)
	assert.Equal(t, ins.expireTime, s.ExpireTime)
	assert.Equal(t, ins.fencingToken, s.FencingToken)
	assert.Equal(t, ins.createRevision, s.CreateRevision)
}
