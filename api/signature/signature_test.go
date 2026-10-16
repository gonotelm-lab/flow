package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// 这个模块（api）只依赖生成的 proto 与少量运行时库，不引入测试框架：
// 上面的 go.work 是本机便利，不能作为「模块自带依赖」的替代。

const (
	testSecret     = "sk-test-secret"
	testTimestamp  = int64(1770000000) // 2026-02-02T02:40:00Z
	testDate       = "20260202"
	testNamespace  = "demo"
	testScheduleID = "018f2f1e-0000-7000-8000-000000000001"
	testFireID     = "018f2f1e-0000-7000-8000-000000000002"
	testBody       = `{"hello":"world"}`
	testSignedList = "content-type;x-flow-fire-id;x-flow-namespace;x-flow-schedule-id;x-flow-scheduled-time;x-flow-timestamp"
)

func testInput(t *testing.T) Input {
	t.Helper()

	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set(HeaderScheduleID, testScheduleID)
	h.Set(HeaderFireID, testFireID)
	h.Set(HeaderScheduledTime, "1770000000000")
	h.Set(HeaderTimestamp, "1770000000")
	h.Set(HeaderNamespace, testNamespace)

	u, err := url.Parse("https://receiver.example.com/hook?a=1&b=2")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	return Input{Method: http.MethodPost, URL: u, Header: h, Body: []byte(testBody)}
}

func mustSign(t *testing.T, secret string, in Input) string {
	t.Helper()
	header, err := Sign(secret, in)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	return header
}

func assertEqual[T comparable](t *testing.T, got, want T, msg string) {
	t.Helper()
	if got != want {
		t.Errorf("%s:\n  got  %v\n  want %v", msg, got, want)
	}
}

func assertErrorIs(t *testing.T, err, target error, msg string) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Errorf("%s: got %v, want it to wrap %v", msg, err, target)
	}
}

// 用标准库独立重算一遍，确保签名规则与包注释里写的一致（而不是自证）。
func TestSign_MatchesDocumentedConstruction(t *testing.T) {
	in := testInput(t)

	canonical := strings.Join([]string{
		"POST",
		"/hook",
		"a=1&b=2",
		"content-type:application/json\n" +
			"x-flow-fire-id:" + testFireID + "\n" +
			"x-flow-namespace:" + testNamespace + "\n" +
			"x-flow-schedule-id:" + testScheduleID + "\n" +
			"x-flow-scheduled-time:1770000000000\n" +
			"x-flow-timestamp:1770000000\n",
		testSignedList,
		sha256Hex([]byte(testBody)),
	}, "\n")

	canonicalSum := sha256.Sum256([]byte(canonical))
	scope := testDate + "/" + testNamespace + "/flow_request"
	stringToSign := strings.Join([]string{
		Algorithm,
		"20260202T024000Z",
		scope,
		hex.EncodeToString(canonicalSum[:]),
	}, "\n")

	kDate := hmacSum([]byte("FLOW"+testSecret), testDate)
	kNamespace := hmacSum(kDate, testNamespace)
	kSigning := hmacSum(kNamespace, scopeFinal)
	want := hex.EncodeToString(hmacSum(kSigning, stringToSign))

	got := mustSign(t, testSecret, in)
	assertEqual(t, got,
		"FLOW-HMAC-SHA256 Scope="+scope+", Signed="+testSignedList+", Signature="+want,
		"签名值必须与文档里写的手算过程一致")

	if err := Verify(testSecret, in, got); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestSign_IsDeterministicAndCoversTheRequestShape(t *testing.T) {
	in := testInput(t)
	base := mustSign(t, testSecret, in)
	assertEqual(t, mustSign(t, testSecret, in), base, "同样的请求必须签出同样的值")

	cases := map[string]func(*Input){
		"路径":           func(i *Input) { i.URL, _ = url.Parse("https://receiver.example.com/other?a=1&b=2") },
		"查询串":          func(i *Input) { i.URL, _ = url.Parse("https://receiver.example.com/hook?a=1&b=3") },
		"方法":           func(i *Input) { i.Method = http.MethodPut },
		"body":         func(i *Input) { i.Body = []byte(testBody + " ") },
		"fire id":      func(i *Input) { i.Header.Set(HeaderFireID, "018f2f1e-0000-7000-8000-000000000003") },
		"空 body":       func(i *Input) { i.Body = nil },
		"content-type": func(i *Input) { i.Header.Set("Content-Type", "text/plain") },
	}
	for name, mutate := range cases {
		other := testInput(t)
		mutate(&other)
		assertEqual(t, mustSign(t, testSecret, other) == base, false, name+"变化必须改变签名")
	}

	assertEqual(t, mustSign(t, "sk-other-secret", in) == base, false, "密钥变化必须改变签名")
}

func TestVerify_RejectsTampering(t *testing.T) {
	in := testInput(t)
	header := mustSign(t, testSecret, in)

	cases := map[string]func(*Input){
		"body 被改":        func(i *Input) { i.Body = []byte(`{"hello":"evil"}`) },
		"路径被改":           func(i *Input) { i.URL, _ = url.Parse("https://receiver.example.com/evil?a=1&b=2") },
		"查询串被改":          func(i *Input) { i.URL, _ = url.Parse("https://receiver.example.com/hook?a=1&b=2&c=3") },
		"方法被改":           func(i *Input) { i.Method = http.MethodGet },
		"schedule id 被改": func(i *Input) { i.Header.Set(HeaderScheduleID, "018f2f1e-0000-7000-8000-000000000009") },
		"fire id 被改":     func(i *Input) { i.Header.Set(HeaderFireID, "018f2f1e-0000-7000-8000-000000000009") },
		"scheduled time 被改": func(i *Input) {
			i.Header.Set(HeaderScheduledTime, "1770000000001")
		},
		"时间戳被改":           func(i *Input) { i.Header.Set(HeaderTimestamp, "1770000001") },
		"content-type 被改": func(i *Input) { i.Header.Set("Content-Type", "text/plain") },
		"secret 不对":       func(i *Input) {},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			tampered := testInput(t)
			mutate(&tampered)
			secret := testSecret
			if name == "secret 不对" {
				secret = "sk-wrong"
			}
			assertErrorIs(t, Verify(secret, tampered, header), ErrMismatch, "必须拒绝")
		})
	}

	// namespace 同时在 scope 与头里，改了会被 scope 一致性检查先拦下。
	tampered := testInput(t)
	tampered.Header.Set(HeaderNamespace, "other")
	assertErrorIs(t, Verify(testSecret, tampered, header), ErrScopeMismatch, "namespace 必须被拒绝")
}

// scope 里的日期必须与时间戳对得上，否则「拿另一天的派生密钥签」这条缝就存在。
func TestVerify_RejectsScopeDateNotMatchingTimestamp(t *testing.T) {
	in := testInput(t)
	header := mustSign(t, testSecret, in)

	shifted := strings.Replace(header, testDate+"/"+testNamespace, "19990101/"+testNamespace, 1)
	if shifted == header {
		t.Fatal("测试自身没构造出差异")
	}
	assertErrorIs(t, Verify(testSecret, in, shifted), ErrScopeMismatch, "scope 日期必须与时间戳一致")
}

// 「少签一个必需头」必须被拒绝：否则签名方可以在 Signed 清单里去掉 timestamp，
// 接收方就再也看不出重放。
func TestVerify_RequiresClosedSignedHeaderList(t *testing.T) {
	in := testInput(t)
	header := mustSign(t, testSecret, in)

	withoutTimestamp := strings.Replace(header, ";x-flow-timestamp", "", 1)
	if withoutTimestamp == header {
		t.Fatal("测试自身没构造出差异")
	}
	assertErrorIs(t, Verify(testSecret, in, withoutTimestamp), ErrScopeMismatch,
		"Signed 清单必须包含全部必需头")
}

func TestVerify_RejectsMalformedHeader(t *testing.T) {
	in := testInput(t)
	header := mustSign(t, testSecret, in)
	head := header[:strings.Index(header, ", Signature=")+len(", Signature=")]

	scoped := testDate + "/" + testNamespace + "/flow_request"
	cases := map[string]string{
		"空值":                 "",
		"只有空白":               "   ",
		"没有参数":               "FLOW-HMAC-SHA256",
		"算法不同":               "HMAC-SHA256 " + header[strings.Index(header, " ")+1:],
		"参数没有等号":             header + ", garbage",
		"未知参数":               header + ", Extra=1",
		"scope 缺分量":          strings.Replace(header, scoped, "20260202/demo", 1),
		"scope 终止分量不同":       strings.Replace(header, scoped, "20260202/demo/other_request", 1),
		"scope 日期非法":         strings.Replace(header, testDate, "2026-02-02", 1),
		"scope namespace 为空": strings.Replace(header, scoped, testDate+"//flow_request", 1),
		"签名不是 hex":           head + "nothex",
		"签名长度不对":             head + "abcdef",
		"Signed 清单为空":        strings.Replace(header, testSignedList, "", 1),
		"Signed 清单未排序":       strings.Replace(header, testSignedList, "x-flow-timestamp;content-type", 1),
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			err := Verify(testSecret, in, value)
			if err == nil {
				t.Fatal("必须报错")
			}
			classifiable := errors.Is(err, ErrMalformedSignature) ||
				errors.Is(err, ErrUnsupportedAlgorithm) ||
				errors.Is(err, ErrScopeMismatch) ||
				errors.Is(err, ErrMissingHeader)
			if !classifiable {
				t.Errorf("错误必须能归类，实际: %v", err)
			}
		})
	}
}

func TestSign_RequiresEverySignedHeader(t *testing.T) {
	for _, name := range SignedHeaderNames {
		t.Run(name, func(t *testing.T) {
			in := testInput(t)
			in.Header.Del(name)

			_, err := Sign(testSecret, in)
			assertErrorIs(t, err, ErrMissingHeader, "缺少 "+name+" 必须报错")
		})
	}
}

func TestSign_RequiresTimestampToBeParseable(t *testing.T) {
	in := testInput(t)
	in.Header.Set(HeaderTimestamp, "not-a-number")
	_, err := Sign(testSecret, in)
	assertErrorIs(t, err, ErrMalformedSignature, "时间戳不是数字必须报错")

	in = testInput(t)
	in.Header.Del(HeaderTimestamp)
	_, err = Sign(testSecret, in)
	assertErrorIs(t, err, ErrMissingHeader, "没有时间戳必须报错")
}

func TestSign_RequiresMethod(t *testing.T) {
	in := testInput(t)
	in.Method = ""
	_, err := Sign(testSecret, in)
	assertErrorIs(t, err, ErrMalformedSignature, "没有方法必须报错")
}

func TestTimestamp(t *testing.T) {
	h := http.Header{}

	h.Set(HeaderTimestamp, "1770000000")
	if ts, err := Timestamp(h); err != nil || ts != testTimestamp {
		t.Errorf("got (%d, %v), want (%d, nil)", ts, err, testTimestamp)
	}

	h.Set(HeaderTimestamp, " 1770000000 ")
	if ts, err := Timestamp(h); err != nil || ts != testTimestamp {
		t.Errorf("带空白也要能解析，got (%d, %v)", ts, err)
	}

	h.Set(HeaderTimestamp, "0")
	if _, err := Timestamp(h); !errors.Is(err, ErrMalformedSignature) {
		t.Errorf("0 不是合法时间戳，got %v", err)
	}

	h.Set(HeaderTimestamp, "abc")
	if _, err := Timestamp(h); !errors.Is(err, ErrMalformedSignature) {
		t.Errorf("非数字必须报错，got %v", err)
	}

	h.Del(HeaderTimestamp)
	if _, err := Timestamp(h); !errors.Is(err, ErrMissingHeader) {
		t.Errorf("缺失必须报错，got %v", err)
	}
}

// 派生链的每一层都要真的起作用。
func TestSigningKey_DependsOnEveryComponent(t *testing.T) {
	base := signingKey(testSecret, testDate, testNamespace)

	if string(base) == string(signingKey(testSecret, "20260203", testNamespace)) {
		t.Error("日期必须参与派生")
	}
	if string(base) == string(signingKey(testSecret, testDate, "other")) {
		t.Error("namespace 必须参与派生")
	}
	if string(base) == string(signingKey("sk-other", testDate, testNamespace)) {
		t.Error("secret 必须参与派生")
	}
	if string(base) == testSecret {
		t.Error("派生密钥不能就是 secret 本身")
	}
	if len(base) != sha256.Size {
		t.Errorf("派生密钥长度 %d, want %d", len(base), sha256.Size)
	}
}

// 头部出现连续空白时两侧折叠规则必须一致（代理改写 Content-Type 的常见形态）。
func TestVerify_ToleratesInsignificantHeaderWhitespace(t *testing.T) {
	in := testInput(t)
	in.Header.Set("Content-Type", "application/json;  charset=utf-8")
	header := mustSign(t, testSecret, in)

	in.Header.Set("Content-Type", " application/json; charset=utf-8 ")
	if err := Verify(testSecret, in, header); err != nil {
		t.Errorf("空白差异不应影响验签: %v", err)
	}
}

func TestCanonicalQueryString_SortsAndEncodes(t *testing.T) {
	u, err := url.Parse("https://h/hook?b=2&a=1&a=0&c=%2Fslash&d=a+b&e")
	if err != nil {
		t.Fatal(err)
	}

	// 名升序，同名按值升序；RFC3986 编码：空格 %20、'/' %2F、空值也要保留 '='。
	assertEqual(t, canonicalQueryString(u), "a=0&a=1&b=2&c=%2Fslash&d=a%20b&e=", "查询串规范化")
}

func TestCanonicalURI(t *testing.T) {
	assertEqual(t, canonicalURI(nil), "/", "nil URL")

	u, _ := url.Parse("https://h")
	assertEqual(t, canonicalURI(u), "/", "空路径")

	u, _ = url.Parse("https://h/a%20b/c")
	assertEqual(t, canonicalURI(u), "/a%20b/c", "保留发送方的百分号编码")
}

// namespace 只校验「非空」，所以它可能带逗号、空格、% —— 这些都不能把
// X-Flow-Signature 的语法（逗号分参数、空格分算法）撑破。
func TestSign_HandlesNamespacesThatWouldBreakTheHeader(t *testing.T) {
	for _, namespace := range []string{
		"demo",
		"demo prod",
		"demo,prod",
		"100%",
		"a/b",
		"café",
	} {
		t.Run(namespace, func(t *testing.T) {
			in := testInput(t)
			in.Header.Set(HeaderNamespace, namespace)

			header := mustSign(t, testSecret, in)
			if err := Verify(testSecret, in, header); err != nil {
				t.Fatalf("namespace %q 必须可签可验: %v", namespace, err)
			}

			// Scope 段本身不能含逗号或空格。
			scopeParam := header[strings.Index(header, "Scope=")+len("Scope="):]
			scopeParam = scopeParam[:strings.Index(scopeParam, ",")]
			if strings.ContainsAny(scopeParam, " \t,") {
				t.Errorf("Scope %q 含分隔符，会撑破签名头语法", scopeParam)
			}
		})
	}
}

// 控制字符进不了 HTTP 头，签名阶段就要报错，而不是等传输层报一个含糊的错。
func TestSign_RejectsNamespaceWithControlCharacter(t *testing.T) {
	in := testInput(t)
	in.Header.Set(HeaderNamespace, "bad\nname")

	_, err := Sign(testSecret, in)
	assertErrorIs(t, err, ErrMalformedSignature, "控制字符必须被拒")
}

// 同一个请求只认一种合法写法：重复参数、非规范的 scope 编码都拒绝，
// 避免「同一份 header 在不同实现里被读出不同语义」。
func TestVerify_RejectsDuplicateAndNonCanonicalParameters(t *testing.T) {
	scope := testDate + "/" + testNamespace + "/flow_request"

	t.Run("重复参数", func(t *testing.T) {
		in := testInput(t)
		header := mustSign(t, testSecret, in)

		for _, extra := range []string{
			", Scope=" + scope,
			", Signed=content-type",
			", Signature=" + strings.Repeat("a", 64),
		} {
			assertErrorIs(t, Verify(testSecret, in, header+extra), ErrMalformedSignature,
				"重复参数必须被拒: "+extra)
		}
	})

	t.Run("scope 非规范编码", func(t *testing.T) {
		in := testInput(t)
		in.Header.Set(HeaderNamespace, "demo prod")
		header := mustSign(t, testSecret, in)

		raw := strings.Replace(header,
			testDate+"/demo%20prod/flow_request", testDate+"/demo prod/flow_request", 1)
		if raw == header {
			t.Fatal("测试自身没构造出差异")
		}
		assertErrorIs(t, Verify(testSecret, in, raw), ErrMalformedSignature, "非规范 scope 必须被拒")
	})
}

func TestEmptyBodyIsLegal(t *testing.T) {
	in := testInput(t)
	in.Body = nil

	header := mustSign(t, testSecret, in)
	if err := Verify(testSecret, in, header); err != nil {
		t.Fatalf("空 body 必须可签可验: %v", err)
	}

	// 空 body 与全空白 body 不是一回事。
	other := testInput(t)
	other.Body = []byte(" ")
	header2 := mustSign(t, testSecret, other)
	if header2 == header {
		t.Error("空 body 与含内容 body 的签名必须不同")
	}
}

func hmacSum(key []byte, msg string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msg))
	return mac.Sum(nil)
}
