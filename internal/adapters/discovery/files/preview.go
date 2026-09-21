package files

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/internal/domain/ingestion"
)

func (adapter Adapter) Preview(ctx context.Context, name string, content []byte) (ingestion.Preview, error) {
	// Validate the entire document using the same limits and active-content policy as ingestion.
	snapshot, err := adapter.Discover(ctx, discovery.Input{Locator: "artifact:preview", ObservedAt: time.Now().UTC(), Files: map[string][]byte{name: content}})
	if err != nil {
		return ingestion.Preview{}, err
	}
	p := ingestion.Preview{Kind: adapter.kind, Lines: []ingestion.PreviewLine{}, Blocks: []ingestion.PreviewBlock{}, Sheets: []ingestion.PreviewSheet{}}
	for _, dataset := range snapshot.Datasets {
		item := ingestion.PreviewDataset{Name: dataset.QualifiedName, Fields: []ingestion.PreviewField{}}
		for i, field := range dataset.Fields {
			if i >= 50 {
				p.Truncated = true
				break
			}
			item.Fields = append(item.Fields, ingestion.PreviewField{Name: field.Name, DataType: field.DataType, Nullable: field.Nullable, Ordinal: field.Ordinal})
		}
		p.Datasets = append(p.Datasets, item)
	}
	budget := 128 << 10
	bounded := func(value string, limit int) string {
		n := min(len(value), limit, budget)
		for n > 0 && !utf8.ValidString(value[:n]) {
			n--
		}
		if n < len(value) {
			p.Truncated = true
		}
		budget -= n
		return value[:n]
	}
	if adapter.kind == ingestion.ArtifactMarkdown {
		p.Text = bounded(string(content), 64<<10)
		lines := strings.Split(strings.TrimSuffix(p.Text, "\n"), "\n")
		for i, line := range lines {
			if i == 500 {
				p.Truncated = true
				break
			}
			p.Lines = append(p.Lines, ingestion.PreviewLine{Number: i + 1, Text: line})
		}
		document := goldmark.New().Parser().Parse(text.NewReader([]byte(p.Text)))
		err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if err := ctx.Err(); err != nil {
				return ast.WalkStop, err
			}
			if !entering || node.Type() != ast.TypeBlock || node.Lines().Len() == 0 {
				return ast.WalkContinue, nil
			}
			kind := "paragraph"
			switch node.Kind() {
			case ast.KindHeading:
				kind = "heading"
			case ast.KindParagraph, ast.KindTextBlock:
			case ast.KindFencedCodeBlock, ast.KindCodeBlock:
				kind = "code"
			default:
				return ast.WalkContinue, nil
			}
			if len(p.Blocks) >= 500 {
				p.Truncated = true
				return ast.WalkStop, nil
			}
			segment := node.Lines().At(0)
			value := string(node.Text([]byte(p.Text)))
			p.Blocks = append(p.Blocks, ingestion.PreviewBlock{Kind: kind, Line: bytes.Count([]byte(p.Text[:segment.Start]), []byte{'\n'}) + 1, Text: value})
			return ast.WalkSkipChildren, nil
		})
		if err != nil {
			return p, err
		}
		return p, nil
	}
	addRow := func(sheet *ingestion.PreviewSheet, number int, values []string) {
		if len(sheet.Rows) >= 100 || budget <= 0 {
			sheet.Truncated = true
			p.Truncated = true
			return
		}
		cells := make([]string, min(len(values), 50))
		if len(values) > 50 {
			sheet.Truncated = true
			p.Truncated = true
		}
		for i := range cells {
			cells[i] = bounded(values[i], 1024)
			if cells[i] != values[i] {
				sheet.Truncated = true
			}
		}
		sheet.Rows = append(sheet.Rows, ingestion.PreviewRow{Number: number, Cells: cells})
	}
	if adapter.kind == ingestion.ArtifactCSV {
		reader := csv.NewReader(bytes.NewReader(content))
		reader.FieldsPerRecord = -1
		headers, _ := reader.Read()
		sheet := ingestion.PreviewSheet{Name: name, Columns: headers[:min(len(headers), 50)], Rows: []ingestion.PreviewRow{}, Truncated: len(headers) > 50}
		for number := 2; ; number++ {
			if err := ctx.Err(); err != nil {
				return p, err
			}
			row, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return p, err
			}
			addRow(&sheet, number, row)
			if len(sheet.Rows) >= 100 || budget <= 0 {
				if _, extraErr := reader.Read(); extraErr != io.EOF {
					sheet.Truncated = true
					p.Truncated = true
				}
				break
			}
		}
		p.Truncated = p.Truncated || sheet.Truncated
		p.Sheets = append(p.Sheets, sheet)
		return p, nil
	}
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return p, err
	}
	files := map[string]*zip.File{}
	for _, file := range archive.File {
		files[file.Name] = file
	}
	refs, err := validatePackageXML(files)
	if err != nil {
		return p, err
	}
	shared, err := loadSharedStrings(ctx, files["xl/sharedStrings.xml"])
	if err != nil {
		return p, err
	}
	totalRows, totalCells := 0, 0
	for _, ref := range refs {
		sheet := ingestion.PreviewSheet{Name: ref.Name, Columns: []string{}, Rows: []ingestion.PreviewRow{}}
		headers, err := parseSheet(ctx, files[ref.Path], shared, &totalRows, &totalCells, func(number int, values []string) {
			if number > 1 {
				addRow(&sheet, number, values)
			}
		})
		if err != nil {
			return p, err
		}
		sheet.Columns = headers[:min(len(headers), 50)]
		for _, row := range sheet.Rows {
			if len(row.Cells) > len(sheet.Columns) {
				sheet.Truncated = true
			}
		}
		if len(headers) > 50 {
			sheet.Truncated = true
		}
		p.Truncated = p.Truncated || sheet.Truncated
		p.Sheets = append(p.Sheets, sheet)
	}
	return p, nil
}
