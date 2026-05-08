package tenanter

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"sync"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/go-yaml/yaml"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

const osEnvKey = "CLOUD_FITTER_CONFIGS"

var (
	ErrLoadTenanterFromFile  = errors.New("load tenanter from file failed")
	ErrLoadTenanterFromOsEnv = errors.New("load tenanter from os env failed")
	ErrLoadTenanterFileEmpty = errors.New("load tenanter from file failed")
	ErrNoTenanters           = errors.New("no tenanters for the cloud")
)

type Tenanter interface {
	AccountName() string
	Clone() Tenanter
}

var gStore = globalStore{stores: make(map[pbtenant.CloudProvider][]Tenanter)}

type globalStore struct {
	sync.Mutex
	stores map[pbtenant.CloudProvider][]Tenanter
}

func LoadCloudConfigs(configFile string) error {
	if err := LoadCloudConfigsFromFile(configFile); errors.Is(err, ErrLoadTenanterFileEmpty) {
		return LoadCloudConfigsFromOsEnv()
	}
	return nil
}

func LoadCloudConfigsFromFile(configFile string) error {
	b, err := ioutil.ReadFile(configFile)
	if err != nil {
		return ErrLoadTenanterFileEmpty
	}

	var raw rawCloudConfigsFile
	if err = yaml.Unmarshal(b, &raw); err != nil {
		return errors.WithMessage(ErrLoadTenanterFromFile, err.Error())
	}
	configs, huaweiScopeByName := raw.toProtoAndHuaweiScopeMap()

	return load(configs, huaweiScopeByName)
}

func LoadCloudConfigsFromOsEnv() error {
	data := os.Getenv(osEnvKey)
	var raw rawCloudConfigsFile
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return errors.WithMessage(ErrLoadTenanterFromOsEnv, err.Error())
	}
	configs, huaweiScopeByName := raw.toProtoAndHuaweiScopeMap()

	return load(configs, huaweiScopeByName)
}

func ShowConfigJson() ([]byte, error) {
	data := os.Getenv(osEnvKey)
	var raw rawCloudConfigsFile
	if err := yaml.Unmarshal([]byte(data), &raw); err != nil {
		return nil, errors.WithMessage(ErrLoadTenanterFromFile, err.Error())
	}
	configs, _ := raw.toProtoAndHuaweiScopeMap()
	return json.Marshal(configs)
}

func load(configs *pbtenant.CloudConfigs, huaweiScopeByName map[string]int32) error {
	gStore.Lock()
	defer gStore.Unlock()
	gStore.stores = make(map[pbtenant.CloudProvider][]Tenanter)
	return applyConfigsLocked(configs, huaweiScopeByName)
}

// ReloadFromConfigs 清空内存中的租户映射后按 configs 重新加载（用于 SQLite 或动态更新）。
// huaweiScopeByName 按账号名称映射 0=国内、1=国际；nil 或缺省键视为国内。
func ReloadFromConfigs(configs *pbtenant.CloudConfigs, huaweiScopeByName map[string]int32) error {
	gStore.Lock()
	defer gStore.Unlock()
	gStore.stores = make(map[pbtenant.CloudProvider][]Tenanter)
	return applyConfigsLocked(configs, huaweiScopeByName)
}

func applyConfigsLocked(configs *pbtenant.CloudConfigs, huaweiScopeByName map[string]int32) error {
	var skipped int
	for _, c := range configs.Configs {
		if c.AccessId != "" && c.AccessSecret != "" {
			scope := HuaweiAccountScopeDomestic
			if c.Provider == pbtenant.CloudProvider_huawei && huaweiScopeByName != nil {
				if s, ok := huaweiScopeByName[c.Name]; ok && s == HuaweiAccountScopeInternational {
					scope = HuaweiAccountScopeInternational
				}
			}
			gStore.stores[c.Provider] = append(gStore.stores[c.Provider], NewAccessKeyTenant(c.Name, c.AccessId, c.AccessSecret, scope))
		} else {
			skipped++
			if c.Name != "" {
				glog.Warningf("config skip account %q: empty access_id or access_secret", c.Name)
			}
		}
	}
	for p, tenants := range gStore.stores {
		name := pbtenant.CloudProvider_name[int32(p)]
		if name == "" {
			name = p.String()
		}
		accs := make([]string, 0, len(tenants))
		for _, t := range tenants {
			accs = append(accs, t.AccountName())
		}
		glog.Infof("loaded cloud %s (%d account(s)): %v", name, len(tenants), accs)
	}
	if len(gStore.stores) == 0 {
		glog.Warningf("no cloud accounts loaded (check config: %d entries skipped for missing keys)", skipped)
	}
	return nil
}

func GetTenanters(provider pbtenant.CloudProvider) ([]Tenanter, error) {
	gStore.Lock()
	defer gStore.Unlock()

	if len(gStore.stores[provider]) == 0 {
		return nil, errors.WithMessagef(ErrNoTenanters, "cloud is %v", provider)
	}

	var tenanters = make([]Tenanter, len(gStore.stores[provider]))
	for k := range gStore.stores[provider] {
		tenanters[k] = gStore.stores[provider][k].Clone()
	}
	return tenanters, nil
}

// rawCloudConfigsFile 与 YAML/JSON（含 CLOUD_FITTER_CONFIGS）兼容；华为云可选 huawei_account_scope（yaml）或 huaweiAccountScope（json）。
type rawCloudConfigsFile struct {
	Configs []struct {
		Provider           int32  `json:"provider" yaml:"provider"`
		Name               string `json:"name" yaml:"name"`
		AccessId           string `json:"access_id" yaml:"access_id"`
		AccessSecret       string `json:"access_secret" yaml:"access_secret"`
		HuaweiAccountScope int32  `json:"huaweiAccountScope,omitempty" yaml:"huawei_account_scope,omitempty"`
	} `json:"configs" yaml:"configs"`
}

func (raw rawCloudConfigsFile) toProtoAndHuaweiScopeMap() (*pbtenant.CloudConfigs, map[string]int32) {
	cfg := &pbtenant.CloudConfigs{}
	huaweiScopeByName := make(map[string]int32)
	for _, c := range raw.Configs {
		cfg.Configs = append(cfg.Configs, &pbtenant.CloudConfig{
			Provider:     pbtenant.CloudProvider(c.Provider),
			Name:         c.Name,
			AccessId:     c.AccessId,
			AccessSecret: c.AccessSecret,
		})
		if pbtenant.CloudProvider(c.Provider) != pbtenant.CloudProvider_huawei || c.Name == "" {
			continue
		}
		if c.HuaweiAccountScope == HuaweiAccountScopeInternational {
			huaweiScopeByName[c.Name] = HuaweiAccountScopeInternational
		}
	}
	return cfg, huaweiScopeByName
}
