package main

import "fmt"

type versionData struct {
	Version          string   `json:"version"`
	Commit           string   `json:"commit,omitempty"`
	BuildDate        string   `json:"build_date,omitempty"`
	SupportedSchemas []string `json:"supported_schemas"`
}

func currentVersionData() versionData {
	return versionData{
		Version:   version,
		Commit:    commit,
		BuildDate: date,
		SupportedSchemas: []string{
			"plane.version.v1",
			"plane.config.v1",
			"plane.config.set.v1",
			"plane.auth.status.v1",
			"plane.doctor.v1",
			"plane.resolve.v1",
			"plane.project.list.v1",
			"plane.project.get.v1",
			"plane.state.list.v1",
			"plane.member.list.v1",
			"plane.error.v1",
		},
	}
}

func (a app) cmdVersion(format string) int {
	data := currentVersionData()
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.version.v1", data))
		return exitOK
	}
	fmt.Fprintln(a.stdout, data.Version)
	return exitOK
}
