package debugger

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
)

const reportHTMLTemplate = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>LocalRPG Test Report - {{.ScenarioName}}</title>
<style>
body { font-family: monospace; background: #0c0a09; color: #f5f5f4; margin: 20px; }
h1 { color: #c084fc; }
.card { background: #1c1917; border: 1px solid #292524; border-radius: 8px; padding: 16px; margin-bottom: 16px; }
.passed { color: #4ade80; font-weight: bold; }
.failed { color: #f87171; font-weight: bold; }
table { width: 100%; border-collapse: collapse; }
th, td { text-align: left; padding: 8px; border-bottom: 1px solid #292524; }
th { background: #292524; }
</style>
</head>
<body>
<h1>LocalRPG Test Report</h1>
<div class="card">
  <h2>Scenario: {{.ScenarioName}}</h2>
  <p>Status: {{if .Passed}}<span class="passed">PASSED</span>{{else}}<span class="failed">FAILED</span>{{end}}</p>
  <p>Duration: {{.DurationMs}}ms | Total Actions: {{.TotalActions}}</p>
</div>
<div class="card">
  <h3>Action Executions</h3>
  <table>
    <tr><th>ID</th><th>#</th><th>Action</th><th>Selector</th><th>Duration</th><th>Status</th></tr>
    {{range .Actions}}
    <tr>
      <td>{{.ID}}</td>
      <td>{{.StepIndex}}</td>
      <td>{{.ActionType}}</td>
      <td>{{.Selector}}</td>
      <td>{{.DurationMs}}ms</td>
      <td><span class="{{.Status}}">{{.Status}}</span></td>
    </tr>
    {{end}}
  </table>
</div>
</body>
</html>`

// ExportHTMLReport renders a self-contained HTML report.
func ExportHTMLReport(report TestReport, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	tmpl, err := template.New("report").Parse(reportHTMLTemplate)
	if err != nil {
		return fmt.Errorf("debugger: parse report template: %w", err)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, report)
}

// ExportJSON writes raw execution logs as JSON.
func ExportJSON(v any, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, data, 0644)
}
