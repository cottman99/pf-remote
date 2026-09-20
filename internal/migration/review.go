package migration

import (
	"errors"
	"fmt"
	"strings"
)

func RenderObservationReview(report ObservationReport, locale string) (string, error) {
	if report.SchemaVersion != ReportSchema || (report.Overall != "ready" && report.Overall != "rollback-required" && report.Overall != "incomplete") || len(report.Targets) == 0 {
		return "", errors.New("migration observation report is invalid")
	}
	if locale == "" {
		locale = "zh-CN"
	}
	if locale != "zh-CN" && locale != "en-US" {
		return "", errors.New("migration review locale is unsupported")
	}
	var ready, rollback, incomplete []TargetAssessment
	for _, target := range report.Targets {
		switch target.Status {
		case "ready":
			ready = append(ready, target)
		case "rollback-required":
			rollback = append(rollback, target)
		case "incomplete":
			incomplete = append(incomplete, target)
		default:
			return "", errors.New("migration observation report contains an invalid status")
		}
	}
	if locale == "en-US" {
		return renderEnglishReview(report.Overall, ready, rollback, incomplete), nil
	}
	return renderChineseReview(report.Overall, ready, rollback, incomplete), nil
}

func renderChineseReview(overall string, ready, rollback, incomplete []TargetAssessment) string {
	var output strings.Builder
	output.WriteString("# PF Remote 迁移观察报告\n\n")
	switch overall {
	case "ready":
		output.WriteString("**结论：可以进入受控切换准备。** 本报告中的操作已通过新旧路径对照，但不会自动停用旧路径。\n")
	case "rollback-required":
		output.WriteString("**结论：暂时不要切换。** 至少一项新路径不符合要求，继续使用旧路径。\n")
	default:
		output.WriteString("**结论：证据还不完整。** 继续保留旧路径，补齐观察后再决定。\n")
	}
	renderChineseSection(&output, "可以进入切换准备", ready)
	renderChineseSection(&output, "必须继续使用旧路径", rollback)
	renderChineseSection(&output, "需要补充观察", incomplete)
	output.WriteString("\n## 安全说明\n\n- 本报告只评估证据，没有修改、停止或切换任何服务。\n- 旧路径必须保持可用，直到单独批准切换并完成回退演练。\n")
	return output.String()
}

func renderChineseSection(output *strings.Builder, title string, targets []TargetAssessment) {
	if len(targets) == 0 {
		return
	}
	output.WriteString("\n## " + title + "\n\n")
	for _, target := range targets {
		fmt.Fprintf(output, "- **%s · %s**：%s（证据编号 `%s`）\n", escapeMarkdown(target.ComputerName), escapeMarkdown(target.CapabilityName), chineseReason(target.Code), target.MappingKey)
	}
}

func chineseReason(code string) string {
	switch code {
	case "match":
		return "新旧路径结果一致，并已确认旧路径仍可用"
	case "revocation-match":
		return "两条路径都正确拒绝了已经撤销的操作"
	case "missing-side":
		return "一侧缺少这项操作，无法完成对照"
	case "identity-mismatch":
		return "新旧两侧的电脑或操作名称不一致"
	case "revocation-mismatch":
		return "新路径没有保持撤销结果"
	case "legacy-unavailable":
		return "旧路径本身不可用，无法作为可靠基线"
	case "candidate-failure":
		return "旧路径可用，但新路径失败"
	case "fallback-not-rechecked":
		return "测试新路径后没有再次确认旧路径"
	case "timing-missing":
		return "缺少可比较的体验速度记录"
	case "major-slowdown":
		return "新路径从快速明显下降到缓慢"
	default:
		return "证据状态无法识别，不能切换"
	}
}

func renderEnglishReview(overall string, ready, rollback, incomplete []TargetAssessment) string {
	var output strings.Builder
	output.WriteString("# PF Remote migration observation report\n\n")
	switch overall {
	case "ready":
		output.WriteString("**Decision: ready for controlled cutover preparation.** This report does not disable the old path.\n")
	case "rollback-required":
		output.WriteString("**Decision: do not cut over.** Keep using the old path for at least one affected action.\n")
	default:
		output.WriteString("**Decision: evidence is incomplete.** Keep the old path and repeat the missing observations.\n")
	}
	renderEnglishSection(&output, "Ready for cutover preparation", ready)
	renderEnglishSection(&output, "Keep the old path", rollback)
	renderEnglishSection(&output, "More observation required", incomplete)
	output.WriteString("\n## Safety note\n\n- This report evaluates evidence only. It did not modify, stop, or switch a service.\n- Keep the old path until cutover is separately approved and rollback is rehearsed.\n")
	return output.String()
}

func renderEnglishSection(output *strings.Builder, title string, targets []TargetAssessment) {
	if len(targets) == 0 {
		return
	}
	output.WriteString("\n## " + title + "\n\n")
	for _, target := range targets {
		fmt.Fprintf(output, "- **%s · %s**: %s (evidence `%s`)\n", escapeMarkdown(target.ComputerName), escapeMarkdown(target.CapabilityName), target.Summary, target.MappingKey)
	}
}

func escapeMarkdown(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]",
		"<", "\\<", ">", "\\>", "#", "\\#", "|", "\\|",
	)
	return replacer.Replace(value)
}
