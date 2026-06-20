package instance

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gonotelm-lab/flow/server/internal/repository/schema"
	"github.com/gonotelm-lab/flow/server/pkg/ip"

	"github.com/google/uuid"
)

var (
	hostname      string
	localIP       string
	instanceValue string
)

type InstanceEventType string

func (e InstanceEventType) String() string {
	return string(e)
}

const (
	InstanceEventPut    InstanceEventType = "PUT"
	InstanceEventDelete InstanceEventType = "DELETE"
)

func init() {
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		panic(fmt.Errorf("failed to get hostname: %w", err))
	}

	localIP, err = ip.DetectLocalIP()
	if err != nil {
		panic(fmt.Errorf("failed to detect local IP: %w", err))
	}

	instanceValue = fmt.Sprintf("%s@%s", hostname, localIP)
}

type Instance struct {
	mu sync.RWMutex

	id             int64
	group          string
	key            string
	value          string
	startTime      int64
	expireTime     int64
	fencingToken   int64
	createRevision int64
}

func (i *Instance) GetId() int64 {
	if i != nil {
		i.mu.RLock()
		defer i.mu.RUnlock()
		return i.id
	}

	return 0
}

func (i *Instance) Replace(other *Instance) {
	if i == nil || other == nil {
		return
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	i.id = other.id
	i.group = other.group
	i.key = other.key
	i.value = other.value
	i.startTime = other.startTime
	i.expireTime = other.expireTime
	i.fencingToken = other.fencingToken
	i.createRevision = other.createRevision
}

func (i *Instance) ToSchema() *schema.Instance {
	return &schema.Instance{
		Id:             i.id,
		Group:          i.group,
		Key:            i.key,
		Value:          i.value,
		StartTime:      i.startTime,
		ExpireTime:     i.expireTime,
		FencingToken:   i.fencingToken,
		CreateRevision: i.createRevision,
	}
}

func newInstanceFromSchema(ins *schema.Instance) *Instance {
	if ins == nil {
		return nil
	}

	return &Instance{
		id:             ins.Id,
		group:          ins.Group,
		key:            ins.Key,
		value:          ins.Value,
		startTime:      ins.StartTime,
		expireTime:     ins.ExpireTime,
		fencingToken:   ins.FencingToken,
		createRevision: ins.CreateRevision,
	}
}

func NewInstance(group string, createRevision int64, expiry time.Duration) *Instance {
	group = strings.TrimSpace(group)

	now := time.Now()
	expireTime := now.Add(expiry)
	key := uuid.NewString()
	key = fmt.Sprintf("%s/%s", group, key)
	return &Instance{
		group:          group,
		key:            key,
		value:          instanceValue,
		startTime:      now.UnixMilli(),
		expireTime:     expireTime.UnixMilli(),
		fencingToken:   rand.Int63(),
		createRevision: createRevision,
	}
}

func (i *Instance) SetExpireTime(expireTimeMs int64) {
	i.expireTime = expireTimeMs
}

type InstanceEvent struct {
	Revision   int64             `json:"revision"`
	Group      string            `json:"group"`
	Key        string            `json:"key"`
	Value      string            `json:"value"`
	EventType  InstanceEventType `json:"event_type"`
	CreateTime int64             `json:"create_time"`
}

type cancellableInstance struct {
	*Instance
	cancel    context.CancelFunc
	parentCtx context.Context
}
