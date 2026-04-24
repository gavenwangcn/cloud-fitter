package cmdb

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Client 对应 cmdb-sync 中 CMBDAPI 的 HTTP 与签名方式。
type Client struct {
	BaseURL  string
	Key      string
	Secret   string
	HTTP     *http.Client
	basePath string
}

func NewClient(baseURL, key, secret string) *Client {
	b := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return &Client{
		BaseURL:  b,
		Key:      key,
		Secret:   secret,
		HTTP:     &http.Client{Timeout: 120 * time.Second},
		basePath: "",
	}
}

func isScalarForSign(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return false
	default:
		return true
	}
}

// BuildAPIKey 与 Python: path + secret + 排序后的参数值拼接，再 SHA1 十六进制。
func (c *Client) BuildAPIKey(path string, params map[string]any) {
	var keys []string
	for k := range params {
		if k == "_key" || k == "_secret" {
			continue
		}
		if !isScalarForSign(params[k]) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var values strings.Builder
	for _, k := range keys {
		values.WriteString(fmt.Sprint(params[k]))
	}
	raw := path + c.Secret + values.String()
	h := sha1.Sum([]byte(raw))
	params["_secret"] = hex.EncodeToString(h[:])
	params["_key"] = c.Key
}

// GetCI GET /api/v0.1/ci/s
func (c *Client) GetCI(q map[string]any) (map[string]any, error) {
	u, err := url.Parse(c.BaseURL + "/api/v0.1/ci/s")
	if err != nil {
		return nil, err
	}
	p := make(map[string]any)
	for k, v := range q {
		p[k] = v
	}
	c.BuildAPIKey(u.Path, p)
	qv := u.Query()
	for _, k := range sortedKeys(p) {
		qv.Set(k, fmt.Sprint(p[k]))
	}
	u.RawQuery = qv.Encode()
	return c.doJSON("GET", u.String(), "application/json", nil)
}

// GetCIFirst 查询 CI 列表取第一条记录（与 GetCIID 同条件时的完整属性）。
func (c *Client) GetCIFirst(q map[string]any) (map[string]any, error) {
	data, err := c.GetCI(q)
	if err != nil {
		return nil, err
	}
	res, _ := data["result"].([]any)
	if len(res) == 0 {
		return nil, nil
	}
	first, _ := res[0].(map[string]any)
	if first == nil {
		return nil, nil
	}
	return first, nil
}

// GetCIID 与 Python get_ci_id：取 result[0]._id
func (c *Client) GetCIID(q map[string]any) (string, error) {
	data, err := c.GetCI(q)
	if err != nil {
		return "", err
	}
	res, _ := data["result"].([]any)
	if len(res) == 0 {
		return "", nil
	}
	first, _ := res[0].(map[string]any)
	if first == nil {
		return "", nil
	}
	id, _ := first["_id"]
	if id == nil {
		return "", nil
	}
	return fmt.Sprint(id), nil
}

// SystemIDExistsInCMDB 分页查询 CMDB 中 _type:system 的 CI，判断是否存在给定 system_id（与全量同步列举方式一致）。
func (c *Client) SystemIDExistsInCMDB(systemID string) (bool, error) {
	if c == nil {
		return false, fmt.Errorf("cmdb client is nil")
	}
	target := strings.TrimSpace(systemID)
	if target == "" {
		return false, nil
	}
	page := 1
	for {
		data, err := c.GetCI(map[string]any{
			"q":    "_type:system",
			"page": page,
		})
		if err != nil {
			return false, err
		}
		res, _ := data["result"].([]any)
		if len(res) == 0 {
			return false, nil
		}
		for _, it := range res {
			row, _ := it.(map[string]any)
			if row == nil {
				continue
			}
			if fmt.Sprint(row["system_id"]) == target {
				return true, nil
			}
		}
		page++
	}
}

// GetSystemLevelRelations GET /api/v0.1/ci_relations/s
func (c *Client) GetSystemLevelRelations(q map[string]any) (map[string]any, error) {
	u, err := url.Parse(c.BaseURL + "/api/v0.1/ci_relations/s")
	if err != nil {
		return nil, err
	}
	p := make(map[string]any)
	for k, v := range q {
		p[k] = v
	}
	c.BuildAPIKey(u.Path, p)
	qv := u.Query()
	for _, k := range sortedKeys(p) {
		qv.Set(k, fmt.Sprint(p[k]))
	}
	u.RawQuery = qv.Encode()
	return c.doJSON("GET", u.String(), "application/json", nil)
}

// AddCI POST /api/v0.1/ci
func (c *Client) AddCI(body map[string]any) (map[string]any, error) {
	return c.postCIBody(body)
}

// UpdateCI 对已存在 CI 做属性更新：POST /api/v0.1/ci，body 含 _id 及待更新字段（与 add_ci 签名方式一致）。
func (c *Client) UpdateCI(_id string, fields map[string]any) (map[string]any, error) {
	if strings.TrimSpace(_id) == "" {
		return nil, fmt.Errorf("update ci: empty _id")
	}
	p := make(map[string]any, len(fields)+1)
	for k, v := range fields {
		p[k] = v
	}
	p["_id"] = _id
	return c.postCIBody(p)
}

func (c *Client) postCIBody(p map[string]any) (map[string]any, error) {
	u, err := url.Parse(c.BaseURL + "/api/v0.1/ci")
	if err != nil {
		return nil, err
	}
	c.BuildAPIKey(u.Path, p)
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return c.doJSON("POST", u.String(), "application/json", b)
}

func (c *Client) doJSON(method, fullURL, contentType string, body []byte) (map[string]any, error) {
	req, err := http.NewRequest(method, fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if method == "POST" && len(body) > 0 {
		req.Header.Set("Content-Type", contentType)
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cmdb %s %s: status %d: %s", method, fullURL, resp.StatusCode, string(raw))
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("cmdb json: %w body=%q", err, string(raw))
	}
	return m, nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
