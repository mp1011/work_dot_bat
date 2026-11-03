package monitor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// ProcessInfo represents a running operating system process.
type ProcessInfo struct {
	PID         int    `json:"pid"`
	CommandLine string `json:"commandLine"`
}

// ProcessLister returns a snapshot of currently running processes.
type ProcessLister func(ctx context.Context) ([]ProcessInfo, error)

// DefaultProcessLister queries the OS for running processes and their command lines.
func DefaultProcessLister(ctx context.Context) ([]ProcessInfo, error) {
	if runtime.GOOS == "windows" {
		return listProcessesWindows(ctx)
	}
	return listProcessesUnix(ctx)
}

type winCimProc struct {
	ProcessId   int    `json:"ProcessId"`
	CommandLine string `json:"CommandLine"`
}

func listProcessesWindows(ctx context.Context) ([]ProcessInfo, error) {
	// Query Win32_Process using PowerShell
	psCmd := "Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -ne $null } | Select-Object ProcessId, CommandLine | ConvertTo-Json -Compress"
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("powershell process list: %w", err)
	}

	data := bytes.TrimSpace(out.Bytes())
	if len(data) == 0 {
		return nil, nil
	}

	// Output might be an array or a single object
	var procs []winCimProc
	if err := json.Unmarshal(data, &procs); err != nil {
		var single winCimProc
		if err2 := json.Unmarshal(data, &single); err2 != nil {
			return nil, fmt.Errorf("parse powershell json: %w", err)
		}
		procs = []winCimProc{single}
	}

	res := make([]ProcessInfo, 0, len(procs))
	for _, p := range procs {
		res = append(res, ProcessInfo{
			PID:         p.ProcessId,
			CommandLine: p.CommandLine,
		})
	}
	return res, nil
}

func listProcessesUnix(ctx context.Context) ([]ProcessInfo, error) {
	cmd := exec.CommandContext(ctx, "ps", "-eo", "pid,args")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ps -eo pid,args: %w", err)
	}

	var res []ProcessInfo
	scanner := bufio.NewScanner(bytes.NewReader(out))
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			// Skip header line if present ("PID ARGS" or "PID COMMAND")
			if strings.HasPrefix(strings.ToUpper(line), "PID") {
				continue
			}
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		cmdLine := strings.TrimSpace(line[len(parts[0]):])
		res = append(res, ProcessInfo{
			PID:         pid,
			CommandLine: cmdLine,
		})
	}
	return res, nil
}
