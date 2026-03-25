package iac

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParsedResource is the normalised representation of a Terraform-managed
// resource extracted from a .tfstate file. It is the contract between the
// state reader and any downstream enumerator / differ.
type ParsedResource struct {
	Address    string                 // e.g. "azurerm_resource_group.example"
	Type       string                 // e.g. "azurerm_resource_group"
	Name       string                 // e.g. "example"
	Mode       string                 // "managed" | "data"
	Provider   string                 // clean name: "aws" | "azurerm" | "github" …
	Id         string                 // value of attributes["id"] — primary lookup key
	Attributes map[string]interface{} // full attribute baseline for drift comparison
}

// ── internal JSON mirror of the .tfstate v4 schema ───────────────────────────

type tfState struct {
	Version   int          `json:"version"`
	Resources []tfResource `json:"resources"`
}

type tfResource struct {
	Mode      string       `json:"mode"`
	Type      string       `json:"type"`
	Name      string       `json:"name"`
	Provider  string       `json:"provider"`
	Instances []tfInstance `json:"instances"`
}

type tfInstance struct {
	Attributes map[string]interface{} `json:"attributes"`
}

// ── provider name extraction ──────────────────────────────────────────────────
//
// Terraform 0.12 state:  "provider.aws"
// Terraform 0.13+ state: "provider[\"registry.terraform.io/hashicorp/aws\"]"
//
// Both cases → "aws"

func extractProviderName(raw string) string {
	if strings.Contains(raw, "/") {
		// new format — take the last path segment and strip the trailing "]
		parts := strings.Split(raw, "/")
		last := parts[len(parts)-1]
		last = strings.TrimSuffix(last, "\"]")
		return last
	}
	// old format — strip "provider." prefix
	return strings.TrimPrefix(raw, "provider.")
}

// ── StateReader ───────────────────────────────────────────────────────────────

func StateReader(path string) ([]ParsedResource, error) {
	reader, err := NewFileReader(path)
	if err != nil {
		return nil, fmt.Errorf("error opening state file: %w", err)
	}
	defer reader.Close()

	var state tfState
	if err := json.NewDecoder(reader).Decode(&state); err != nil {
		return nil, fmt.Errorf("error parsing state file: %w", err)
	}

	var resources []ParsedResource

	for _, res := range state.Resources {
		// mirror driftctl: skip data sources, only process managed resources
		if res.Mode != "managed" {
			continue
		}

		provider := extractProviderName(res.Provider)

		for _, instance := range res.Instances {
			id, _ := instance.Attributes["id"].(string)

			resources = append(resources, ParsedResource{
				Address:    res.Type + "." + res.Name,
				Type:       res.Type,
				Name:       res.Name,
				Mode:       res.Mode,
				Provider:   provider,
				Id:         id,
				Attributes: instance.Attributes,
			})
		}
	}

	return resources, nil
}
