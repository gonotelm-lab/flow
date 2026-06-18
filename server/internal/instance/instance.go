package instance

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/gonotelm-lab/flow/server/pkg/ip"
	"github.com/gonotelm-lab/flow/server/internal/repository/schema"

	"github.com/google/uuid"
)

var (
	hostname      string
	localIP       string
	instanceValue string
)

const (
	// 默认10s过期时间
	defaultExpiry            = time.Second * 12
	defaultKeepaliveInterval = time.Second * 10
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

const (
	InstanceGroup = "flow/instances"
)

type Instance struct {
	Id             int64  `json:"id"`
	Group          string `json:"group"`
	Key            string `json:"key"`
	Value          string `json:"value"`
	StartTime      int64  `json:"start_time"`
	ExpireTime     int64  `json:"expire_time"`
	FencingToken   int64  `json:"fencing_token"`
	CreateRevision int64  `json:"create_revision"`
}

func (i *Instance) ToSchema() *schema.Instance {
	return &schema.Instance{
		Id:             i.Id,
		Group:          i.Group,
		Key:            i.Key,
		Value:          i.Value,
		StartTime:      i.StartTime,
		ExpireTime:     i.ExpireTime,
		FencingToken:   i.FencingToken,
		CreateRevision: i.CreateRevision,
	}
}

func (i *Instance) Duplicate() *Instance {
	di := *i
	return &di
}

func NewInstance(createRevision int64) *Instance {
	now := time.Now()
	expireTime := now.Add(defaultExpiry)
	key := uuid.NewString()
	key = fmt.Sprintf("%s/%s", InstanceGroup, key)
	return &Instance{
		Group:          InstanceGroup,
		Key:            key,
		Value:          instanceValue,
		StartTime:      now.UnixMilli(),
		ExpireTime:     expireTime.UnixMilli(),
		FencingToken:   rand.Int63(),
		CreateRevision: createRevision,
	}
}

func (i *Instance) ExtendTTL(ttl time.Duration) {
	i.ExpireTime = time.UnixMilli(i.ExpireTime).Add(ttl).UnixMilli()
}

func (i *Instance) SetExpireTime(expireTimeMs int64) {
	i.ExpireTime = expireTimeMs
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
	cancel context.CancelFunc
}
