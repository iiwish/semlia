package governance

import (
	"strings"
	"unicode/utf8"

	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	distributiondomain "github.com/iiwish/semlia/internal/domain/distribution"
)

const (
	askClarificationLead  = "这个问题没能对应到已发布知识（可能是指标未发布、需要的计算暂不支持、问题含写入类操作，或描述不够具体），系统不会猜测。"
	askClarificationClose = "请改用这些指标和属性提问，并写明时间范围或比较对象。"
	askNothingPublished   = "当前没有你有权使用的已发布知识，暂时无法回答这个问题。请联系管理员发布知识或授予使用权限。"
	// The API schema allows 500 characters; keep a margin for the fixed sentences.
	askClarificationLimit = 500
	askMenuNameLimit      = 24
)

type askMenuObject struct {
	name    string
	members []string
}

// askClarificationText is server-owned: it never echoes the question or model prose. It names only what the caller
// is already authorized to read in the released knowledge, so the same release always yields the same text and a
// replay can rebuild it without storing anything extra.
func askClarificationText(catalog []distributionapp.AskCatalogEntry) string {
	if len(catalog) == 0 {
		return askNothingPublished
	}
	var metrics []string
	var objects []askMenuObject
	seenMetric, seenObject := map[string]bool{}, map[string]bool{}
	for _, entry := range catalog {
		name := askMenuName(entry.Name)
		if name == "" {
			continue
		}
		switch entry.Type {
		case "metric":
			if !seenMetric[name] {
				seenMetric[name] = true
				metrics = append(metrics, name)
			}
		case "business_object":
			if seenObject[name] {
				continue
			}
			seenObject[name] = true
			object := askMenuObject{name: name}
			for _, member := range entry.Members {
				if memberName := askMenuName(member); memberName != "" {
					object.members = append(object.members, memberName)
				}
			}
			if len(object.members) > 0 {
				objects = append(objects, object)
			}
		}
	}
	if len(metrics) == 0 && len(objects) == 0 {
		return askClarificationLead + askClarificationClose
	}
	for metricCap, objectCap, memberCap := 8, 5, 6; ; {
		text := askClarificationLead + "当前可用：" + askMenu(metrics, objects, metricCap, objectCap, memberCap) + "。" + askClarificationClose
		if utf8.RuneCountInString(text) <= askClarificationLimit || metricCap <= 1 && objectCap <= 1 && memberCap <= 1 {
			return text
		}
		metricCap, objectCap, memberCap = max(1, metricCap-1), max(1, objectCap-1), max(1, memberCap-1)
	}
}

func askMenu(metrics []string, objects []askMenuObject, metricCap, objectCap, memberCap int) string {
	var parts []string
	if len(metrics) > 0 {
		shown := metrics[:min(len(metrics), metricCap)]
		parts = append(parts, "指标 "+strings.Join(shown, "、")+askMore(len(metrics) > len(shown)))
	}
	if len(objects) > 0 {
		shown := objects[:min(len(objects), objectCap)]
		items := make([]string, 0, len(shown))
		for _, object := range shown {
			members := object.members[:min(len(object.members), memberCap)]
			items = append(items, object.name+"（"+strings.Join(members, "、")+askMore(len(object.members) > len(members))+"）")
		}
		parts = append(parts, "对象与属性 "+strings.Join(items, "、")+askMore(len(objects) > len(shown)))
	}
	return strings.Join(parts, "；")
}

func askMore(truncated bool) string {
	if truncated {
		return "等"
	}
	return ""
}

func askMenuName(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) > askMenuNameLimit {
		runes := []rune(value)
		value = string(runes[:askMenuNameLimit]) + "…"
	}
	return value
}

const askDisambiguationClose = "请在问题中写明指哪一个。"

var askTypeLabels = map[string]string{"metric": "指标", "business_object": "业务对象", "business_term": "业务口径", "data_asset": "数据资产", "analysis_model": "分析模型"}

// askDisambiguationText is server-owned like the menu: it names only candidates the caller may read and the
// distinctions their authors published, and is rebuilt from the same release on replay.
func askDisambiguationText(ambiguities []distributiondomain.AskAmbiguity) string {
	for distinctionCap := 60; ; distinctionCap -= 15 {
		parts := []string{}
		for _, ambiguity := range ambiguities {
			label := askTypeLabels[string(ambiguity.Type)]
			if label == "" {
				label = "知识"
			}
			candidates := []string{}
			for _, candidate := range ambiguity.Candidates {
				item := askMenuName(candidate.Name)
				if distinction := strings.Join(strings.Fields(candidate.Distinction), " "); distinction != "" && distinctionCap > 0 {
					if runes := []rune(distinction); len(runes) > distinctionCap {
						distinction = string(runes[:distinctionCap]) + "…"
					}
					item += "（" + distinction + "）"
				}
				candidates = append(candidates, item)
			}
			parts = append(parts, "「"+askMenuName(ambiguity.Text)+"」可能指不同的"+label+"："+strings.Join(candidates, "；"))
		}
		text := strings.Join(parts, "。") + "。" + askDisambiguationClose
		if utf8.RuneCountInString(text) <= askClarificationLimit {
			return text
		}
		if distinctionCap <= 0 {
			runes := []rune(text)
			return string(runes[:askClarificationLimit-utf8.RuneCountInString(askDisambiguationClose)-1]) + "…" + askDisambiguationClose
		}
	}
}

// askServerClarification prefers a disambiguation the release can state over the generic menu.
func askServerClarification(knowledge distributionapp.AskKnowledge) string {
	if len(knowledge.Ambiguities) > 0 {
		return askDisambiguationText(knowledge.Ambiguities)
	}
	return askClarificationText(knowledge.Catalog)
}
