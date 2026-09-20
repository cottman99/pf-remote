package migration

import (
	"strings"
	"testing"
)

func TestRenderObservationReview_ChineseKeepsTechnicalKeysSecondary(t *testing.T) {
	report := ObservationReport{
		SchemaVersion: ReportSchema, Overall: "rollback-required",
		Targets: []TargetAssessment{
			{MappingKey: "migration-target-alpha", ComputerName: "实验室电脑", CapabilityName: "当前屏幕", Status: "ready", Code: "match", Summary: "matched"},
			{MappingKey: "migration-target-beta", ComputerName: "工作站", CapabilityName: "自动化", Status: "rollback-required", Code: "candidate-failure", Summary: "failed"},
			{MappingKey: "migration-target-gamma", ComputerName: "旧电脑", CapabilityName: "独立桌面", Status: "incomplete", Code: "timing-missing", Summary: "missing"},
		},
	}
	review, err := RenderObservationReview(report, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"暂时不要切换", "可以进入切换准备", "必须继续使用旧路径", "需要补充观察", "实验室电脑 · 当前屏幕", "旧路径可用，但新路径失败", "证据编号 `migration-target-alpha`", "没有修改、停止或切换任何服务"} {
		if !strings.Contains(review, expected) {
			t.Fatalf("review does not contain %q:\n%s", expected, review)
		}
	}
	if strings.Index(review, "实验室电脑 · 当前屏幕") > strings.Index(review, "migration-target-alpha") {
		t.Fatal("technical mapping key appeared before the user-facing action")
	}
}

func TestRenderObservationReview_EnglishAndValidation(t *testing.T) {
	report := ObservationReport{SchemaVersion: ReportSchema, Overall: "ready", Targets: []TargetAssessment{{MappingKey: "migration-target-alpha", ComputerName: "Lab computer", CapabilityName: "Current screen", Status: "ready", Code: "match", Summary: "matched"}}}
	review, err := RenderObservationReview(report, "en-US")
	if err != nil || !strings.Contains(review, "ready for controlled cutover preparation") {
		t.Fatalf("review=%q err=%v", review, err)
	}
	if _, err := RenderObservationReview(report, "fr-FR"); err == nil {
		t.Fatal("unsupported locale was accepted")
	}
	report.Overall = "unknown"
	if _, err := RenderObservationReview(report, "zh-CN"); err == nil {
		t.Fatal("invalid report was accepted")
	}
}

func TestRenderObservationReview_EscapesUserFacingMarkdown(t *testing.T) {
	report := ObservationReport{SchemaVersion: ReportSchema, Overall: "ready", Targets: []TargetAssessment{{MappingKey: "migration-target-alpha", ComputerName: "**fake decision**", CapabilityName: "[click](bad)", Status: "ready", Code: "match", Summary: "matched"}}}
	review, err := RenderObservationReview(report, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(review, "****fake") || !strings.Contains(review, "\\*\\*fake decision\\*\\*") || !strings.Contains(review, "\\[click\\](bad)") {
		t.Fatalf("unsafe Markdown review:\n%s", review)
	}
}
