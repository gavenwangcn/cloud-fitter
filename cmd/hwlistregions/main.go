// Command hwlistregions lists Huawei Cloud IAM regions using AK/SK (SDK request signing).
//
// Usage (PowerShell):
//
//	$env:HUAWEI_ACCESS_KEY_ID="your-ak"
//	$env:HUAWEI_SECRET_ACCESS_KEY="your-sk"
//	go run ./cmd/hwlistregions
//	go run ./cmd/hwlistregions -all
//	go run ./cmd/hwlistregions -raw
//
// Optional: $env:HUAWEI_IAM_ENDPOINT="https://iam.ru-moscow-1.myhuaweicloud.com"
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	hwiam "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3"
	iammodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/iam/v3/model"

	"github.com/cloud-fitter/cloud-fitter/internal/huaweicloudregion"
)

func main() {
	all := flag.Bool("all", false, "print all regions (id + zh-cn + en-us)")
	raw := flag.Bool("raw", false, "print full KeystoneListRegionsResponse JSON (links + regions)")
	flag.Parse()

	ak := strings.TrimSpace(os.Getenv("HUAWEI_ACCESS_KEY_ID"))
	sk := strings.TrimSpace(os.Getenv("HUAWEI_SECRET_ACCESS_KEY"))
	if ak == "" || sk == "" {
		fmt.Fprintln(os.Stderr, "set HUAWEI_ACCESS_KEY_ID and HUAWEI_SECRET_ACCESS_KEY")
		os.Exit(2)
	}
	endpoint := strings.TrimSpace(os.Getenv("HUAWEI_IAM_ENDPOINT"))
	if endpoint == "" {
		endpoint = "https://iam.myhuaweicloud.com"
	}

	auth, err := global.NewCredentialsBuilder().WithAk(ak).WithSk(sk).SafeBuild()
	if err != nil {
		fmt.Fprintln(os.Stderr, "credentials:", err)
		os.Exit(1)
	}

	hc := hwiam.IamClientBuilder().
		WithEndpoint(endpoint).
		WithCredential(auth).
		WithHttpConfig(huaweicloudregion.SDKHttpConfig()).
		Build()
	client := hwiam.NewIamClient(hc)

	resp, err := client.KeystoneListRegions(&iammodel.KeystoneListRegionsRequest{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "KeystoneListRegions:", err)
		os.Exit(1)
	}

	if *raw {
		b, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "marshal:", err)
			os.Exit(1)
		}
		os.Stdout.Write(b)
		os.Stdout.WriteString("\n")
		return
	}

	if resp.Regions == nil {
		fmt.Println("no regions in response")
		return
	}
	fmt.Fprintf(os.Stderr, "total regions: %d\n", len(*resp.Regions))

	type row struct {
		ID      string            `json:"id"`
		Locales map[string]string `json:"locales,omitempty"`
	}
	allRows := make([]row, 0, len(*resp.Regions))
	for _, r := range *resp.Regions {
		loc := map[string]string{}
		if r.Locales != nil {
			if r.Locales.ZhCn != "" {
				loc["zh-cn"] = r.Locales.ZhCn
			}
			if r.Locales.EnUs != "" {
				loc["en-us"] = r.Locales.EnUs
			}
		}
		allRows = append(allRows, row{ID: r.Id, Locales: loc})
	}

	if *all {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(allRows); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	out := make([]row, 0, 8)
	ruLike := make([]row, 0, 8)
	for _, item := range allRows {
		id := item.ID
		joined := strings.ToLower(id + " " + item.Locales["zh-cn"] + " " + item.Locales["en-us"])
		if strings.Contains(joined, "moscow") || strings.Contains(joined, "莫斯科") || strings.Contains(joined, "ru-moscow") || strings.Contains(joined, "russia") || strings.Contains(joined, "俄罗斯") || strings.Contains(joined, "op4") {
			out = append(out, item)
		}
		if strings.HasPrefix(strings.ToLower(id), "ru-") {
			ruLike = append(ruLike, item)
		}
	}
	if len(out) == 0 && len(ruLike) > 0 {
		out = ruLike
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
