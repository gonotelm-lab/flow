// Package e2e 是跑在真 flowserver + 真 PostgreSQL 上的端到端测试。
//
// 默认跳过：只有设置了 FLOW_E2E_ADDR 才会跑。本地用法：
//
//	# 起库与 server（见 README「端到端测试」），然后：
//	cd client && FLOW_E2E_ADDR=127.0.0.1:7091 go test ./e2e/... -v
//
// 环境变量：
//
//	FLOW_E2E_ADDR       flowserver 的 gRPC 地址，例如 127.0.0.1:7091（设置它才会跑）
//	FLOW_E2E_ADMIN_URL  Admin HTTP 地址，默认 http://127.0.0.1:7090（用来建 namespace 拿 key）
//	FLOW_E2E_AUTH_MODE  enabled（默认）或 disabled：server 端是否开着 api_key 鉴权
//
// 每个用例通过 Admin API 建一个本次运行专用的 namespace（没有删除 namespace 的接口，
// 所以名字带时间戳，反复跑不会撞）。
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	envAddr     = "FLOW_E2E_ADDR"
	envAdminURL = "FLOW_E2E_ADMIN_URL"
	envAuthMode = "FLOW_E2E_AUTH_MODE"

	defaultAdminURL = "http://127.0.0.1:7090"
	authModeEnabled = "enabled"
)

// requireE2E 在没配 FLOW_E2E_ADDR 时跳过，保证 `go test ./...` 在没有 server 的机器上照常通过。
func requireE2E(t *testing.T) {
	t.Helper()

	if os.Getenv(envAddr) == "" {
		t.Skip("FLOW_E2E_ADDR 未配置，跳过端到端测试")
	}
}

func serverAddr(t *testing.T) string {
	t.Helper()
	requireE2E(t)

	return os.Getenv(envAddr)
}

func adminURL(t *testing.T) string {
	t.Helper()

	if url := os.Getenv(envAdminURL); url != "" {
		return url
	}

	return defaultAdminURL
}

// authEnabled 报告服务端是否开着鉴权；默认按开着处理，避免 CI 忘了设环境变量时静默漏测。
func authEnabled(t *testing.T) bool {
	t.Helper()

	mode := os.Getenv(envAuthMode)
	if mode == "" {
		return true
	}

	require.Contains(t, []string{authModeEnabled, "disabled"}, mode, "FLOW_E2E_AUTH_MODE 取值非法")

	return mode == authModeEnabled
}

// newNamespace 建一个专用 namespace，返回名字与明文 api_key（只在创建响应里出现一次）。
func newNamespace(t *testing.T) (string, string) {
	t.Helper()
	requireE2E(t)

	name := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	body, err := json.Marshal(map[string]any{
		"namespace": map[string]string{"name": name},
	})
	require.NoError(t, err)

	resp, err := http.Post(adminURL(t)+"/api/admin/v1/namespaces", "application/json", bytes.NewReader(body))
	require.NoError(t, err, "调用 Admin API 建 namespace 失败")
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode, "建 namespace 应当返回 200")

	var created struct {
		Name   string `json:"name"`
		APIKey string `json:"apiKey"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.Equal(t, name, created.Name)
	require.NotEmpty(t, created.APIKey, "创建响应必须带明文 api_key")

	return created.Name, created.APIKey
}
