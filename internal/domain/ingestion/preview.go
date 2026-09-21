package ingestion

import (
	"context"
	"errors"
)

var ErrContentUnavailable = errors.New("artifact content is unavailable")

// Preview is a bounded, inert projection of an immutable artifact, never executable content.
type Preview struct {
	ArtifactID    string           `json:"artifactId"`
	ContentDigest string           `json:"contentDigest"`
	Kind          ArtifactKind     `json:"kind"`
	Text          string           `json:"text"`
	Lines         []PreviewLine    `json:"lines"`
	Blocks        []PreviewBlock   `json:"blocks"`
	Datasets      []PreviewDataset `json:"datasets"`
	Sheets        []PreviewSheet   `json:"sheets"`
	Truncated     bool             `json:"truncated"`
}
type PreviewDataset struct {
	Name   string         `json:"name"`
	Fields []PreviewField `json:"fields"`
}
type PreviewField struct {
	Name     string `json:"name"`
	DataType string `json:"dataType"`
	Nullable bool   `json:"nullable"`
	Ordinal  int    `json:"ordinal"`
}
type PreviewBlock struct {
	Kind string `json:"kind"`
	Line int    `json:"line"`
	Text string `json:"text"`
}
type PreviewLine struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}
type PreviewRow struct {
	Number int      `json:"number"`
	Cells  []string `json:"cells"`
}
type PreviewSheet struct {
	Name      string       `json:"name"`
	Columns   []string     `json:"columns"`
	Rows      []PreviewRow `json:"rows"`
	Truncated bool         `json:"truncated"`
}
type ContentPreviewer interface {
	Preview(context.Context, string, []byte) (Preview, error)
}
