package checks

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// Gates on `npm audit` reporting high/critical severity vulnerabilities.
var failSeverities = []string{"high", "critical"}

type AuditViolation struct{ Reason string }

// EvaluateAudit reads an `npm audit --json` report.
func EvaluateAudit(report []byte) (passed bool, violations []AuditViolation, err error) {
	var r struct {
		Metadata struct {
			Vulnerabilities map[string]int `json:"vulnerabilities"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(report, &r); err != nil {
		return false, nil, err
	}
	for _, sev := range failSeverities {
		if n := r.Metadata.Vulnerabilities[sev]; n > 0 {
			noun := "vulnerabilities"
			if n == 1 {
				noun = "vulnerability"
			}
			violations = append(violations, AuditViolation{fmt.Sprintf(
				"%d %s severity %s found (run `npm audit` for details, `npm audit fix` to attempt an automatic fix)", n, sev, noun)})
		}
	}
	return len(violations) == 0, violations, nil
}

// RunNpmAudit returns the JSON report; npm exits non-zero when vulnerabilities are found but the
// report is still on stdout.
func RunNpmAudit(dir string) ([]byte, error) {
	cmd := exec.Command("npm", "audit", "--json")
	cmd.Dir = dir
	out, err := cmd.Output()
	var ee *exec.ExitError
	if err != nil && errors.As(err, &ee) && len(out) > 0 {
		return out, nil
	}
	return out, err
}

// AuditMain is `check-audit [check]`.
func AuditMain(root string, argv []string, out, errOut io.Writer) int {
	if len(argv) > 0 && argv[0] != "check" {
		fmt.Fprintf(errOut, "Unknown command: \"%s\"\nUsage: check-audit [check]\n", argv[0])
		return 1
	}
	report, err := RunNpmAudit(root)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 1
	}
	passed, violations, err := EvaluateAudit(report)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 1
	}
	if passed {
		fmt.Fprintln(out, "npm audit gate passed (no high or critical severity vulnerabilities).")
		return 0
	}
	fmt.Fprintf(out, "npm audit gate found %d violation(s):\n", len(violations))
	for _, v := range violations {
		fmt.Fprintf(out, "  ❌ %s\n", v.Reason)
	}
	return 1
}
