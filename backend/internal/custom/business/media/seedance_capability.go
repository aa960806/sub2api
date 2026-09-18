package media

import "strings"

// Seedance 模型 ID。只使用上游 catalog 的四个展示 id，不接受带下划线的旧写法。
const (
	SeedanceModel25       = "seedance2.5"
	SeedanceModel20       = "seedance2.0"
	SeedanceModel20Fast   = "seedance2.0fast"
	SeedanceModel20Mini   = "seedance2.0mini"
	SeedanceResolution720 = "720p"
)

// seedanceModelCapability 是单个 Seedance 模型的能力约束。
// 数据来源为上游 GET /v1/catalog 快照。
type seedanceModelCapability struct {
	Durations    []int
	MaxRefImages int
}

// seedanceCapabilities 固定四个模型的合法时长与参考图张数上限。
// 上游 catalog 变更时必须同步本表并重新验收。
var seedanceCapabilities = map[string]seedanceModelCapability{
	SeedanceModel25:     {Durations: []int{30}, MaxRefImages: 30},
	SeedanceModel20:     {Durations: []int{5, 10, 15}, MaxRefImages: 9},
	SeedanceModel20Fast: {Durations: []int{5, 10, 15}, MaxRefImages: 9},
	SeedanceModel20Mini: {Durations: []int{5, 10}, MaxRefImages: 9},
}

// IsSeedanceModel 报告 model 是否为受支持的 Seedance 模型 ID。
func IsSeedanceModel(model string) bool {
	_, ok := seedanceCapabilities[strings.TrimSpace(model)]
	return ok
}

// SeedanceModelIDs 返回受支持的 Seedance 模型 ID（顺序固定，供能力发现接口渲染）。
func SeedanceModelIDs() []string {
	return []string{SeedanceModel25, SeedanceModel20, SeedanceModel20Fast, SeedanceModel20Mini}
}

// SeedanceModelDurations 返回该模型的合法时长；模型未知时返回 nil。
func SeedanceModelDurations(model string) []int {
	cap, ok := seedanceCapabilities[strings.TrimSpace(model)]
	if !ok {
		return nil
	}
	out := make([]int, len(cap.Durations))
	copy(out, cap.Durations)
	return out
}

// SeedanceMaxReferenceImages 返回该模型的参考图张数上限；模型未知时返回 0。
func SeedanceMaxReferenceImages(model string) int {
	return seedanceCapabilities[strings.TrimSpace(model)].MaxRefImages
}

// IsValidSeedanceDuration 报告 model + duration 是否为合法组合。
// 上游对非法组合返回 422，此处前置拦截避免无谓的上游往返与计费冻结。
func IsValidSeedanceDuration(model string, durationSeconds int) bool {
	cap, ok := seedanceCapabilities[strings.TrimSpace(model)]
	if !ok {
		return false
	}
	for _, d := range cap.Durations {
		if d == durationSeconds {
			return true
		}
	}
	return false
}
