package protocol

import "strings"

func ExtractBranch(fullUsername string) (string, string) {
	if !strings.Contains(fullUsername, "@") {
		return fullUsername, "master"
	}

	parts := strings.Split(fullUsername, "@")
	if len(parts) == 2 && parts[1] == "" {
		return parts[0], "master"
	}

	return parts[0], parts[1]
}
