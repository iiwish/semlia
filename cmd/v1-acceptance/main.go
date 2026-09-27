// v1-acceptance emits synthetic input templates, never writes application state.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/iiwish/semlia/internal/demodata"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func buildFixture() demodata.Scenario {
	scenario := demodata.New()
	scenario.Snapshot.WorkspaceID, _ = identity.NewWorkspaceID()
	for i := range scenario.Snapshot.Assets {
		asset := &scenario.Snapshot.Assets[i]
		var content map[string]any
		if err := json.Unmarshal(asset.Content, &content); err != nil {
			panic("invalid built-in synthetic fixture")
		}
		content["scope"] = "合成验收；2026 年 7 月至 8 月 UTC；仅固定样本，不代表真实业务"
		if asset.AssetType == semantic.AnalysisModel {
			content["definition"] = "合成验收分析模型：订单按客户标识关联客户，采用多对一左连接；支持有效支付金额、有效订单数及客单价，按 UTC 支付时间和客户当前区域分析。老客户由查询起始时间与全历史首次有效支付时间判断。"
		}
		asset.Content, _ = json.Marshal(content)
	}
	return scenario
}

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: v1-acceptance (emit synthetic fixture)")
		os.Exit(2)
	}
	if err := json.NewEncoder(os.Stdout).Encode(buildFixture()); err != nil {
		fmt.Fprintln(os.Stderr, "Cannot emit synthetic fixture")
		os.Exit(1)
	}
}
