package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

func buildOSReleasePromptContext() string {
	if runtime.GOOS != "linux" {
		return ""
	}

	path := strings.TrimSpace(os.Getenv("BASHCORRECT_OS_RELEASE_PATH"))
	if path == "" {
		path = "/etc/os-release"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return ""
	}
	if len(content) > 4000 {
		content = content[:4000] + "\n# truncated"
	}

	return fmt.Sprintf("OS release data (cat /etc/os-release):\n%s\n", content)
}
