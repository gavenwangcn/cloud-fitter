package tenanter

// 华为云账号区域（仅 provider=huawei 有意义）：0=国内（多区域聚合），1=俄罗斯 ru-moscow-1，2=土耳其（伊斯坦布尔）tr-west-1。
const (
	HuaweiAccountScopeDomestic int32 = 0
	HuaweiAccountScopeRussia   int32 = 1
	HuaweiAccountScopeTurkey   int32 = 2
)

type AccessKeyTenant struct {
	name                string
	id                  string
	secret              string
	huaweiAccountScope  int32
}

func NewTenantWithAccessKey(name, accessKeyId, accessKeySecret string) Tenanter {
	return NewAccessKeyTenant(name, accessKeyId, accessKeySecret, HuaweiAccountScopeDomestic)
}

// NewAccessKeyTenant 创建带华为区域类型的租户；非华为云账号可传 0。
func NewAccessKeyTenant(name, accessKeyId, accessKeySecret string, huaweiAccountScope int32) Tenanter {
	if huaweiAccountScope != HuaweiAccountScopeRussia && huaweiAccountScope != HuaweiAccountScopeTurkey {
		huaweiAccountScope = HuaweiAccountScopeDomestic
	}
	return &AccessKeyTenant{
		name:               name,
		id:                 accessKeyId,
		secret:             accessKeySecret,
		huaweiAccountScope: huaweiAccountScope,
	}
}

// HuaweiAccountScope 返回 0=国内、1=俄罗斯、2=土耳其（非 AccessKeyTenant 实现可视为 0）。
func (tenant *AccessKeyTenant) HuaweiAccountScope() int32 {
	return tenant.huaweiAccountScope
}

func (tenant *AccessKeyTenant) AccountName() string {
	return tenant.name
}

func (tenant *AccessKeyTenant) Clone() Tenanter {
	return &AccessKeyTenant{
		id:                 tenant.id,
		secret:             tenant.secret,
		name:               tenant.name,
		huaweiAccountScope: tenant.huaweiAccountScope,
	}
}

func (tenant *AccessKeyTenant) GetId() string {
	return tenant.id
}

func (tenant *AccessKeyTenant) GetSecret() string {
	return tenant.secret
}
