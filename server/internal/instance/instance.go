package instance

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
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

func newInstanceFromSchema(ins *schema.Instance) *Instance {
	if ins == nil {
		return nil
	}

	return &Instance{
		Id:             ins.Id,
		Group:          ins.Group,
		Key:            ins.Key,
		Value:          ins.Value,
		StartTime:      ins.StartTime,
		ExpireTime:     ins.ExpireTime,
		FencingToken:   ins.FencingToken,
		CreateRevision: ins.CreateRevision,
	}
}

func (i *Instance) Duplicate() *Instance {
	di := *i
	return &di
}

func NewInstance(group string, createRevision int64, expiry time.Duration) *Instance {
	group = strings.TrimSpace(group)

	now := time.Now()
	expireTime := now.Add(expiry)
	key := uuid.NewString()
	key = fmt.Sprintf("%s/%s", group, key)
	return &Instance{
		Group:          group,
		Key:            key,
		Value:          instanceValue,
		StartTime:      now.UnixMilli(),
		ExpireTime:     expireTime.UnixMilli(),
		FencingToken:   rand.Int63(),
		CreateRevision: createRevision,
	}
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
	cancel    context.CancelFunc
	parentCtx context.Context
}
