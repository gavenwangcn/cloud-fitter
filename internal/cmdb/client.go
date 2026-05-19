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

// Client 封装 CMDB 开放 API 的 HTTP 与签名。
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
	case map[string]any, []any, []string:
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

// GetSystemNameBySystemID 按 system_id 查询 CMDB system 的 system_name。
func (c *Client) GetSystemNameBySystemID(systemID string) (string, error) {
	target := strings.TrimSpace(systemID)
	if target == "" {
		return "", nil
	}
	row, err := c.GetCIFirst(map[string]any{
		"q": fmt.Sprintf("_type:system,system_id:%s", target),
	})
	if err != nil {
		return "", err
	}
	if row == nil {
		return "", nil
	}
	return strings.TrimSpace(fmt.Sprint(row["system_name"])), nil
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

// encodeCIFieldValue 将字段编码为 CMDB request.values 标量；is_list 长文本多值用逗号分隔（服务端 handle_arg_list 解析）。
func encodeCIFieldValue(v any) any {
	switch val := v.(type) {
	case []string:
		var parts []string
		for _, s := range val {
			if t := strings.TrimSpace(s); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, ",")
	case []any:
		var parts []string
		for _, x := range val {
			if t := strings.TrimSpace(fmt.Sprint(x)); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, ",")
	default:
		return v
	}
}

func ciParamsFromFields(fields map[string]any) map[string]any {
	p := make(map[string]any, len(fields))
	for k, v := range fields {
		if k == "_id" {
			continue
		}
		p[k] = encodeCIFieldValue(v)
	}
	return p
}

// doCIWithJSONAuth POST/PUT CI：业务字段与 _key/_secret 放在 JSON body。
// CMDB auth_required 在 Content-Type 为 JSON 时将 request.json 赋给 request.values，_wrap_ci_dict 同源读取。
// 多值属性（如 domain_name）须为逗号分隔字符串，不可传 JSON 数组，否则签名字段与服务端 req_args 不一致导致 401。
func (c *Client) doCIWithJSONAuth(method, path string, fields map[string]any) (map[string]any, error) {
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return nil, err
	}
	p := ciParamsFromFields(fields)
	c.BuildAPIKey(u.Path, p)
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return c.doJSON(method, u.String(), "application/json", b)
}

// UpdateCI 对已存在 CI 做属性更新：PUT /api/v0.1/ci/<ci_id>，与 CMDB api.views.cmdb.ci.CIView.put 一致。
// 创建须用 AddCI（POST /api/v0.1/ci）；勿再用 POST 携带 _id 模拟更新，否则会走 CIManager.add 并触发「CI already exists!」。
func (c *Client) UpdateCI(_id string, fields map[string]any) (map[string]any, error) {
	id := strings.TrimSpace(_id)
	if id == "" {
		return nil, fmt.Errorf("update ci: empty _id")
	}
	return c.doCIWithJSONAuth("PUT", "/api/v0.1/ci/"+id, fields)
}

// DeleteCI 硬删除 CI：DELETE /api/v0.1/ci/<ci_id>。
// 服务端：cmdb-api api/views/cmdb/ci.py CIView.delete(ci_id)，仅使用路径参数。
// 鉴权：api/lib/perm/auth.py _auth_with_key 从 request.values 读 _key/_secret；Flask 下查询参数会进入 values，JSON body 不一定参与签名参数合并。
// 前端控制台：cmdb-ui modules/cmdb/api/ci.js deleteCI 为无 body 的 DELETE + Access-Token。
// 本客户端（API Key）：与 GetCI 一致，将 BuildAPIKey 结果挂到 URL query，避免 DELETE + JSON 与鉴权不一致。
func (c *Client) DeleteCI(ciID string) (map[string]any, error) {
	id := strings.TrimSpace(ciID)
	if id == "" {
		return nil, fmt.Errorf("delete ci: empty ci id")
	}
	u, err := url.Parse(c.BaseURL + "/api/v0.1/ci/" + id)
	if err != nil {
		return nil, err
	}
	p := map[string]any{}
	c.BuildAPIKey(u.Path, p)
	qv := u.Query()
	for _, k := range sortedKeys(p) {
		qv.Set(k, fmt.Sprint(p[k]))
	}
	u.RawQuery = qv.Encode()
	return c.doJSON("DELETE", u.String(), "application/json", nil)
}

func (c *Client) postCIBody(p map[string]any) (map[string]any, error) {
	return c.doCIWithJSONAuth("POST", "/api/v0.1/ci", p)
}

func (c *Client) doJSON(method, fullURL, contentType string, body []byte) (map[string]any, error) {
	req, err := http.NewRequest(method, fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if (method == "POST" || method == "PUT" || method == "PATCH") && len(body) > 0 {
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
		errURL := redactCMDBURLSecrets(fullURL)
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("cmdb %s %s: status 401 unauthorized (check CLOUD_FITTER_CMDB_KEY/SECRET; multi-value fields must be comma-separated strings in JSON, not arrays): %s",
				method, errURL, string(raw))
		}
		return nil, fmt.Errorf("cmdb %s %s: status %d: %s", method, errURL, resp.StatusCode, string(raw))
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

func redactCMDBURLSecrets(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	if q.Has("_secret") {
		q.Set("_secret", "***")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
