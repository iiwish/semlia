package files

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	goldmarkutil "github.com/yuin/goldmark/util"
)

const (
	adapterVersion      = "1.0.0"
	maxHeaderBytes      = 255
	maxMarkdownDepth    = 256
	maxMarkdownLinkSize = 2048
)

type Adapter struct{ kind ingestion.ArtifactKind }

func CSV() Adapter      { return Adapter{kind: ingestion.ArtifactCSV} }
func XLSX() Adapter     { return Adapter{kind: ingestion.ArtifactXLSX} }
func Markdown() Adapter { return Adapter{kind: ingestion.ArtifactMarkdown} }

func (adapter Adapter) Kind() string { return "file_" + string(adapter.kind) }
func (Adapter) Version() string      { return adapterVersion }

func (adapter Adapter) Discover(ctx context.Context, input discovery.Input) (discovery.Snapshot, error) {
	if err := input.Validate(); err != nil || len(input.Files) != 1 || ctx.Err() != nil {
		return discovery.Snapshot{}, discovery.ErrInvalidInput
	}
	var name string
	var content []byte
	for name, content = range input.Files {
	}
	snapshot := discovery.Snapshot{AdapterKind: adapter.Kind(), AdapterVersion: adapterVersion,
		ExternalRevision: input.ExternalRevision, Locator: input.Locator, ContentDigest: input.ContentDigest(),
		ObservedAt: input.ObservedAt}
	var err error
	switch adapter.kind {
	case ingestion.ArtifactCSV:
		snapshot.Datasets, err = parseCSV(ctx, name, content)
	case ingestion.ArtifactXLSX:
		snapshot.Datasets, err = parseXLSX(ctx, name, content)
	case ingestion.ArtifactMarkdown:
		snapshot.Datasets, err = parseMarkdown(ctx, name, content)
	default:
		err = ingestion.ErrUnsupported
	}
	if err != nil {
		return discovery.Snapshot{}, err
	}
	snapshot.DeclareCoverage(discovery.PathCoverageKey("file", name), name)
	if err := snapshot.Canonicalize(); err != nil {
		return discovery.Snapshot{}, err
	}
	return snapshot, nil
}

func parseCSV(ctx context.Context, name string, content []byte) ([]discovery.Dataset, error) {
	if len(content) == 0 || int64(len(content)) > ingestion.MaxUploadBytes || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return nil, ingestion.ErrUnsafeContent
	}
	reader := csv.NewReader(bytes.NewReader(content))
	reader.FieldsPerRecord = -1
	var headers []string
	rows, cells, width := 0, 0, -1
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, ingestion.ErrUnsafeContent
		}
		rows++
		if rows > ingestion.MaxRows || len(record) == 0 || len(record) > ingestion.MaxColumns {
			return nil, ingestion.ErrLimitExceeded
		}
		if width < 0 {
			width = len(record)
		} else if len(record) != width {
			return nil, ingestion.ErrUnsafeContent
		}
		cells += len(record)
		if cells > ingestion.MaxCells {
			return nil, ingestion.ErrLimitExceeded
		}
		for _, cell := range record {
			if int64(len(cell)) > ingestion.MaxCellBytes || strings.IndexByte(cell, 0) >= 0 || activeCell(cell) {
				return nil, ingestion.ErrUnsafeContent
			}
		}
		if rows == 1 {
			seenHeaders := make(map[string]struct{}, len(record))
			for _, header := range record {
				header = strings.TrimSpace(header)
				if header == "" || ingestion.ContainsUnsafeDisplayRune(header) {
					return nil, ingestion.ErrUnsafeContent
				}
				if len(header) > maxHeaderBytes {
					return nil, ingestion.ErrLimitExceeded
				}
				if _, duplicate := seenHeaders[header]; duplicate {
					return nil, ingestion.ErrUnsafeContent
				}
				seenHeaders[header] = struct{}{}
			}
			headers = append([]string(nil), record...)
		}
	}
	if rows == 0 || len(headers) == 0 {
		return nil, ingestion.ErrUnsafeContent
	}
	return []discovery.Dataset{fileDataset(name, "csv", headers)}, nil
}

func parseMarkdown(ctx context.Context, name string, content []byte) ([]discovery.Dataset, error) {
	if len(content) == 0 || int64(len(content)) > ingestion.MaxMarkdownBytes || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return nil, ingestion.ErrUnsafeContent
	}
	for _, line := range bytes.Split(content, []byte{'\n'}) {
		if int64(len(line)) > ingestion.MaxCellBytes {
			return nil, ingestion.ErrLimitExceeded
		}
	}
	// Bound the amount of syntax Goldmark can turn into AST nodes before it
	// allocates the tree. The later AST walk remains the exact enforcement.
	syntaxTokens := 1
	for _, value := range content {
		switch value {
		case '\n', '#', '*', '_', '[', ']', '(', ')', '<', '>', '`', '|', '&', ';', '\\':
			syntaxTokens++
		}
		if syntaxTokens > ingestion.MaxMarkdownNodes {
			return nil, ingestion.ErrLimitExceeded
		}
	}
	markdown := goldmark.New(goldmark.WithParserOptions(parser.WithAutoHeadingID()))
	document := markdown.Parser().Parse(text.NewReader(content))
	nodes, depth := 0, 0
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if err := ctx.Err(); err != nil {
			return ast.WalkStop, err
		}
		if !entering {
			depth--
			return ast.WalkContinue, nil
		}
		depth++
		nodes++
		if nodes > ingestion.MaxMarkdownNodes || depth > maxMarkdownDepth || node.Kind() == ast.KindRawHTML || node.Kind() == ast.KindHTMLBlock {
			return ast.WalkStop, ingestion.ErrUnsafeContent
		}
		var destination []byte
		switch value := node.(type) {
		case *ast.Link:
			destination = value.Destination
		case *ast.Image:
			destination = value.Destination
		}
		if len(destination) > maxMarkdownLinkSize {
			return ast.WalkStop, ingestion.ErrLimitExceeded
		}
		normalized := goldmarkutil.URLEscape(bytes.TrimSpace(destination), true)
		if goldmarkhtml.IsDangerousURL(normalized) {
			return ast.WalkStop, ingestion.ErrUnsafeContent
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	return []discovery.Dataset{fileDataset(name, "markdown", []string{"content"})}, nil
}

type xlsxCell struct {
	Reference string `xml:"r,attr"`
	Type      string `xml:"t,attr"`
	Value     string `xml:"v"`
	Inline    struct {
		Text string `xml:"t"`
	} `xml:"is"`
	Formula *struct{} `xml:"f"`
}

type workbookContents struct {
	Defaults []struct {
		Extension   string `xml:"Extension,attr"`
		ContentType string `xml:"ContentType,attr"`
	} `xml:"Default"`
	Overrides []struct {
		PartName    string `xml:"PartName,attr"`
		ContentType string `xml:"ContentType,attr"`
	} `xml:"Override"`
}

type relationships struct {
	Items []struct {
		ID         string `xml:"Id,attr"`
		Target     string `xml:"Target,attr"`
		TargetMode string `xml:"TargetMode,attr"`
		Type       string `xml:"Type,attr"`
	} `xml:"Relationship"`
}

type workbookSheets struct {
	Sheets []struct {
		Name string `xml:"name,attr"`
		ID   string `xml:"id,attr"`
	} `xml:"sheets>sheet"`
}

type worksheetRef struct {
	Name string
	Path string
}

func parseXLSX(ctx context.Context, name string, content []byte) ([]discovery.Dataset, error) {
	if len(content) == 0 || int64(len(content)) > ingestion.MaxUploadBytes || !bytes.HasPrefix(content, []byte{'P', 'K'}) {
		return nil, ingestion.ErrUnsafeContent
	}
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil || len(archive.File) > ingestion.MaxArchiveEntries {
		return nil, ingestion.ErrUnsafeContent
	}
	files := make(map[string]*zip.File, len(archive.File))
	var expanded uint64
	for _, file := range archive.File {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		entryName := file.Name
		isDirectory := strings.HasSuffix(entryName, "/")
		if isDirectory {
			entryName = strings.TrimSuffix(entryName, "/")
		}
		clean := path.Clean(entryName)
		mode := file.Mode()
		if clean != entryName || path.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "\\") ||
			file.Flags&1 != 0 || mode&os.ModeSymlink != 0 || (!mode.IsRegular() && !mode.IsDir()) {
			return nil, ingestion.ErrUnsafeContent
		}
		if isDirectory || mode.IsDir() {
			continue
		}
		if _, duplicate := files[clean]; duplicate {
			return nil, ingestion.ErrUnsafeContent
		}
		size, compressed := file.UncompressedSize64, file.CompressedSize64
		if size > uint64(ingestion.MaxArchiveEntryBytes) || (compressed == 0 && size > 0) ||
			(compressed > 0 && size > uint64(ingestion.MaxArchiveExpansion)*compressed) {
			return nil, ingestion.ErrLimitExceeded
		}
		if expanded > uint64(ingestion.MaxArchiveBytes)-size {
			return nil, ingestion.ErrLimitExceeded
		}
		expanded += size
		lower := strings.ToLower(clean)
		if !safeOOXMLPart(lower) {
			return nil, ingestion.ErrUnsafeContent
		}
		files[clean] = file
	}
	// Inspect every permitted XML part, including unreferenced sheets and table formulas.
	for _, file := range files {
		if err := rejectActivePackageXML(ctx, file); err != nil {
			return nil, err
		}
	}
	if files["[Content_Types].xml"] == nil || files["xl/workbook.xml"] == nil || files["xl/_rels/workbook.xml.rels"] == nil {
		return nil, ingestion.ErrUnsafeContent
	}
	sheets, err := validatePackageXML(files)
	if err != nil {
		return nil, err
	}
	shared, err := loadSharedStrings(ctx, files["xl/sharedStrings.xml"])
	if err != nil {
		return nil, err
	}
	if len(sheets) == 0 || len(sheets) > ingestion.MaxSheets {
		return nil, ingestion.ErrLimitExceeded
	}
	result := make([]discovery.Dataset, 0, len(sheets))
	totalRows, totalCells := 0, 0
	for _, sheet := range sheets {
		headers, err := parseSheet(ctx, files[sheet.Path], shared, &totalRows, &totalCells)
		if err != nil {
			return nil, err
		}
		result = append(result, fileDataset(fmt.Sprintf("%s#%s", name, sheet.Name), "xlsx", headers))
	}
	return result, nil
}

func rejectActivePackageXML(ctx context.Context, file *zip.File) error {
	reader, err := file.Open()
	if err != nil {
		return ingestion.ErrUnsafeContent
	}
	defer reader.Close()
	decoder := xml.NewDecoder(io.LimitReader(reader, ingestion.MaxArchiveEntryBytes+1))
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return ingestion.ErrUnsafeContent
		}
		if start, ok := token.(xml.StartElement); ok {
			switch strings.ToLower(start.Name.Local) {
			case "f", "formula", "formula1", "formula2", "calculatedcolumnformula", "totalsrowformula", "definedname", "oleobject", "control", "activexcontrol", "externalreference", "ddeitem", "ddelink":
				return ingestion.ErrUnsafeContent
			}
		}
	}
}

func validatePackageXML(files map[string]*zip.File) ([]worksheetRef, error) {
	contentTypesBytes, err := readZip(files["[Content_Types].xml"], ingestion.MaxArchiveEntryBytes)
	if err != nil {
		return nil, err
	}
	var contentTypes workbookContents
	if xml.Unmarshal(contentTypesBytes, &contentTypes) != nil {
		return nil, ingestion.ErrUnsafeContent
	}
	for _, fallback := range contentTypes.Defaults {
		if !safeOOXMLDefault(fallback.Extension, fallback.ContentType) {
			return nil, ingestion.ErrUnsafeContent
		}
	}
	mainWorkbook := false
	for _, override := range contentTypes.Overrides {
		part, kind := strings.ToLower(override.PartName), strings.ToLower(override.ContentType)
		part = strings.TrimPrefix(part, "/")
		if !safeOOXMLPart(part) || !hasOOXMLFile(files, part) || !safeOOXMLContentType(kind) {
			return nil, ingestion.ErrUnsafeContent
		}
		if part == "xl/workbook.xml" && kind == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml" {
			mainWorkbook = true
		}
	}
	if !mainWorkbook {
		return nil, ingestion.ErrUnsafeContent
	}
	workbook, err := readZip(files["xl/workbook.xml"], ingestion.MaxArchiveEntryBytes)
	if err != nil {
		return nil, err
	}
	decoder := xml.NewDecoder(bytes.NewReader(workbook))
	for {
		token, tokenErr := decoder.Token()
		if errors.Is(tokenErr, io.EOF) {
			break
		}
		if tokenErr != nil {
			return nil, ingestion.ErrUnsafeContent
		}
		if start, ok := token.(xml.StartElement); ok {
			switch strings.ToLower(start.Name.Local) {
			case "definedname", "workbookprotection", "filesharing":
				return nil, ingestion.ErrUnsafeContent
			}
		}
	}
	var workbookModel workbookSheets
	if xml.Unmarshal(workbook, &workbookModel) != nil || len(workbookModel.Sheets) == 0 {
		return nil, ingestion.ErrUnsafeContent
	}
	var workbookRelations relationships
	for name, file := range files {
		if !strings.HasSuffix(strings.ToLower(name), ".rels") {
			continue
		}
		value, err := readZip(file, ingestion.MaxArchiveEntryBytes)
		if err != nil {
			return nil, err
		}
		var relations relationships
		if xml.Unmarshal(value, &relations) != nil {
			return nil, ingestion.ErrUnsafeContent
		}
		for _, relation := range relations.Items {
			kind := strings.ToLower(relation.Type)
			if strings.EqualFold(strings.TrimSpace(relation.TargetMode), "external") ||
				!safeOOXMLRelationshipType(kind) {
				return nil, ingestion.ErrUnsafeContent
			}
			target, targetErr := relationshipTarget(name, relation.Target)
			if targetErr != nil || files[target] == nil || !safeOOXMLPart(strings.ToLower(target)) {
				return nil, ingestion.ErrUnsafeContent
			}
		}
		if name == "xl/_rels/workbook.xml.rels" {
			workbookRelations = relations
		}
	}
	byID := make(map[string]string, len(workbookRelations.Items))
	for _, relation := range workbookRelations.Items {
		if relation.ID == "" || relation.Target == "" || !strings.Contains(strings.ToLower(relation.Type), "/worksheet") {
			continue
		}
		target := path.Clean(path.Join("xl", relation.Target))
		if path.IsAbs(relation.Target) || strings.Contains(relation.Target, "\\") || !strings.HasPrefix(target, "xl/worksheets/") || files[target] == nil {
			return nil, ingestion.ErrUnsafeContent
		}
		if _, duplicate := byID[relation.ID]; duplicate {
			return nil, ingestion.ErrUnsafeContent
		}
		byID[relation.ID] = target
	}
	result := make([]worksheetRef, 0, len(workbookModel.Sheets))
	seenNames, seenPaths := map[string]struct{}{}, map[string]struct{}{}
	for _, sheet := range workbookModel.Sheets {
		sheet.Name = strings.TrimSpace(sheet.Name)
		target := byID[sheet.ID]
		if sheet.Name == "" || sheet.ID == "" || target == "" || len([]rune(sheet.Name)) > 31 || strings.ContainsAny(sheet.Name, `[]:*?/\`) ||
			ingestion.ContainsUnsafeDisplayRune(sheet.Name) {
			return nil, ingestion.ErrUnsafeContent
		}
		if _, duplicate := seenNames[sheet.Name]; duplicate {
			return nil, ingestion.ErrUnsafeContent
		}
		if _, duplicate := seenPaths[target]; duplicate {
			return nil, ingestion.ErrUnsafeContent
		}
		seenNames[sheet.Name], seenPaths[target] = struct{}{}, struct{}{}
		result = append(result, worksheetRef{Name: sheet.Name, Path: target})
	}
	return result, nil
}

func hasOOXMLFile(files map[string]*zip.File, expected string) bool {
	for name := range files {
		if strings.EqualFold(name, expected) {
			return true
		}
	}
	return false
}

func safeOOXMLPart(name string) bool {
	switch name {
	case "[content_types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels",
		"xl/sharedstrings.xml", "xl/styles.xml", "docprops/core.xml", "docprops/app.xml", "docprops/custom.xml":
		return true
	}
	return numberedOOXMLPart(name, "xl/worksheets/sheet", ".xml") ||
		numberedOOXMLPart(name, "xl/worksheets/_rels/sheet", ".xml.rels") ||
		numberedOOXMLPart(name, "xl/theme/theme", ".xml") ||
		numberedOOXMLPart(name, "xl/tables/table", ".xml")
}

func numberedOOXMLPart(name, prefix, suffix string) bool {
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return false
	}
	number := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	if number == "" {
		return false
	}
	for _, value := range number {
		if value < '0' || value > '9' {
			return false
		}
	}
	return true
}

func safeOOXMLDefault(extension, contentType string) bool {
	extension = strings.ToLower(strings.TrimSpace(extension))
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return (extension == "rels" && contentType == "application/vnd.openxmlformats-package.relationships+xml") ||
		(extension == "xml" && (contentType == "application/xml" || contentType == "text/xml"))
}

func safeOOXMLContentType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sharedstrings+xml",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml",
		"application/vnd.openxmlformats-officedocument.theme+xml",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml",
		"application/vnd.openxmlformats-package.core-properties+xml",
		"application/vnd.openxmlformats-officedocument.extended-properties+xml",
		"application/vnd.openxmlformats-officedocument.custom-properties+xml":
		return true
	default:
		return false
	}
}

func safeOOXMLRelationshipType(value string) bool {
	for _, suffix := range []string{"/officedocument", "/worksheet", "/styles", "/sharedstrings", "/theme", "/table", "/core-properties", "/extended-properties", "/custom-properties"} {
		if strings.HasSuffix(strings.ToLower(strings.TrimSpace(value)), suffix) {
			return true
		}
	}
	return false
}

func relationshipTarget(relationshipFile, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" || path.IsAbs(target) || strings.ContainsAny(target, `\\?#`) {
		return "", ingestion.ErrUnsafeContent
	}
	base := "."
	if relationshipFile != "_rels/.rels" {
		source := strings.TrimSuffix(strings.Replace(relationshipFile, "/_rels/", "/", 1), ".rels")
		base = path.Dir(source)
	}
	resolved := path.Clean(path.Join(base, target))
	if resolved == "." || resolved == ".." || strings.HasPrefix(resolved, "../") {
		return "", ingestion.ErrUnsafeContent
	}
	return resolved, nil
}

func loadSharedStrings(ctx context.Context, file *zip.File) ([]string, error) {
	if file == nil {
		return nil, nil
	}
	reader, err := file.Open()
	if err != nil {
		return nil, ingestion.ErrUnsafeContent
	}
	defer reader.Close()
	decoder := xml.NewDecoder(io.LimitReader(reader, ingestion.MaxArchiveEntryBytes+1))
	result := make([]string, 0)
	var current strings.Builder
	inItem := false
	totalBytes := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, ingestion.ErrUnsafeContent
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "si" {
				if len(result) >= ingestion.MaxCells {
					return nil, ingestion.ErrLimitExceeded
				}
				current.Reset()
				inItem = true
			}
		case xml.CharData:
			if inItem {
				if current.Len()+len(value) > int(ingestion.MaxCellBytes) {
					return nil, ingestion.ErrLimitExceeded
				}
				current.Write(value)
			}
		case xml.EndElement:
			if value.Name.Local == "si" && inItem {
				text := current.String()
				if !utf8.ValidString(text) || strings.IndexByte(text, 0) >= 0 {
					return nil, ingestion.ErrUnsafeContent
				}
				totalBytes += len(text)
				if totalBytes > int(ingestion.MaxArchiveBytes) {
					return nil, ingestion.ErrLimitExceeded
				}
				result = append(result, text)
				inItem = false
			}
		}
	}
	return result, nil
}

func parseSheet(ctx context.Context, file *zip.File, shared []string, totalRows, totalCells *int, visitors ...func(int, []string)) ([]string, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, ingestion.ErrUnsafeContent
	}
	defer reader.Close()
	decoder := xml.NewDecoder(io.LimitReader(reader, ingestion.MaxArchiveEntryBytes+1))
	rowNumber, lastRowCoordinate, lastColumn := 0, 0, 0
	insideRow := false
	var headers []string
	var previewRow []string
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, ingestion.ErrUnsafeContent
		}
		if end, ok := token.(xml.EndElement); ok && end.Name.Local == "row" {
			for _, visit := range visitors {
				visit(rowNumber, previewRow)
			}
			previewRow = nil
			insideRow = false
			continue
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "row":
			if insideRow {
				return nil, ingestion.ErrUnsafeContent
			}
			insideRow = true
			rowNumber++
			rowCoordinate := 0
			for _, attribute := range start.Attr {
				if attribute.Name.Local == "r" {
					coordinate, parseErr := strconv.Atoi(attribute.Value)
					if parseErr != nil || coordinate < 1 || coordinate > ingestion.MaxRows || coordinate <= lastRowCoordinate {
						return nil, ingestion.ErrLimitExceeded
					}
					rowCoordinate = coordinate
				}
			}
			if rowCoordinate == 0 {
				return nil, ingestion.ErrUnsafeContent
			}
			rowNumber = rowCoordinate
			lastRowCoordinate, lastColumn = rowCoordinate, 0
			*totalRows++
			if *totalRows > ingestion.MaxRows {
				return nil, ingestion.ErrLimitExceeded
			}
		case "c":
			if !insideRow {
				return nil, ingestion.ErrUnsafeContent
			}
			var cell xlsxCell
			if err := decoder.DecodeElement(&cell, &start); err != nil {
				return nil, ingestion.ErrUnsafeContent
			}
			column, cellRow, err := spreadsheetCoordinate(cell.Reference)
			if err != nil || cellRow != rowNumber || column <= lastColumn {
				return nil, ingestion.ErrUnsafeContent
			}
			lastColumn = column
			if column > ingestion.MaxColumns {
				return nil, ingestion.ErrLimitExceeded
			}
			*totalCells++
			if *totalCells > ingestion.MaxCells || cell.Formula != nil {
				if cell.Formula != nil {
					return nil, ingestion.ErrUnsafeContent
				}
				return nil, ingestion.ErrLimitExceeded
			}
			value, err := xlsxValue(cell, shared)
			if err != nil || !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 || int64(len(value)) > ingestion.MaxCellBytes {
				return nil, ingestion.ErrUnsafeContent
			}
			if rowNumber == 1 {
				if len(strings.TrimSpace(value)) > maxHeaderBytes {
					return nil, ingestion.ErrLimitExceeded
				}
				for len(headers) < column {
					headers = append(headers, "")
				}
				headers[column-1] = value
			}
			if len(visitors) > 0 {
				for len(previewRow) < column {
					previewRow = append(previewRow, "")
				}
				previewRow[column-1] = value
			}
		}
	}
	if rowNumber == 0 || len(headers) == 0 {
		return nil, ingestion.ErrUnsafeContent
	}
	seenHeaders := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" || len(header) > maxHeaderBytes || ingestion.ContainsUnsafeDisplayRune(header) {
			return nil, ingestion.ErrUnsafeContent
		}
		if _, duplicate := seenHeaders[header]; duplicate {
			return nil, ingestion.ErrUnsafeContent
		}
		seenHeaders[header] = struct{}{}
	}
	return headers, nil
}

func spreadsheetCoordinate(reference string) (int, int, error) {
	reference = strings.ToUpper(strings.TrimSpace(reference))
	column := 0
	digits := false
	row := 0
	for _, value := range reference {
		if value >= 'A' && value <= 'Z' && !digits {
			column = column*26 + int(value-'A'+1)
			continue
		}
		if value >= '0' && value <= '9' {
			digits = true
			row = row*10 + int(value-'0')
			if row > ingestion.MaxRows {
				return 0, 0, ingestion.ErrLimitExceeded
			}
			continue
		}
		return 0, 0, ingestion.ErrUnsafeContent
	}
	if column == 0 || !digits || row == 0 {
		return 0, 0, ingestion.ErrUnsafeContent
	}
	return column, row, nil
}

func xlsxValue(cell xlsxCell, shared []string) (string, error) {
	if cell.Type == "inlineStr" {
		return cell.Inline.Text, nil
	}
	if cell.Type == "s" {
		index, err := strconv.Atoi(cell.Value)
		if err != nil || index < 0 || index >= len(shared) {
			return "", ingestion.ErrUnsafeContent
		}
		return shared[index], nil
	}
	return cell.Value, nil
}

func readZip(file *zip.File, limit int64) ([]byte, error) {
	if file == nil {
		return nil, ingestion.ErrUnsafeContent
	}
	reader, err := file.Open()
	if err != nil {
		return nil, ingestion.ErrUnsafeContent
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil || int64(len(content)) > limit {
		return nil, ingestion.ErrLimitExceeded
	}
	return content, nil
}

func activeCell(value string) bool {
	value = strings.TrimLeft(value, " \t\r\n")
	if value == "" {
		return false
	}
	if value[0] == '-' {
		number, err := strconv.ParseFloat(value, 64)
		return err != nil || math.IsInf(number, 0) || math.IsNaN(number)
	}
	return strings.ContainsRune("=+@", rune(value[0]))
}

func fileDataset(name, kind string, headers []string) discovery.Dataset {
	fileName, sheetName, hasSheet := strings.Cut(name, "#")
	qualified := strings.TrimSuffix(path.Base(fileName), path.Ext(fileName))
	if qualified == "" || qualified == "." {
		qualified = "file"
	}
	if hasSheet {
		qualified += "." + sheetName
	}
	external := "file:" + name
	dataset := discovery.Dataset{ExternalKey: external, QualifiedName: qualified, Kind: "external", Locator: name}
	for index, raw := range headers {
		field := strings.TrimSpace(raw)
		dataset.Fields = append(dataset.Fields, discovery.Field{ExternalKey: external + ":" + field,
			Name: field, Ordinal: index + 1, DataType: kind + "_text", Nullable: true})
	}
	return dataset
}
